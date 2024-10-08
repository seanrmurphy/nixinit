package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pkg/sftp"

	"github.com/gliderlabs/ssh"
	"github.com/pterm/pterm"
	gossh "golang.org/x/crypto/ssh"
)

// NixInitState represents the state of the nixinit server; this can be seen by
// ssh'ing to the server.
type NixInitState int

// add enum which captures the different states of the nixinit server
const (
	WaitingForNixConfig NixInitState = iota
	ConfiguringNixSystem
	ShuttingDown
	NixinitError
	UnableToDetermineInstanceID
)

var (
	port                 = 2222
	host                 = "0.0.0.0"
	validUser            = "nixinit"
	sftpRootDirectory    = ""
	currentState         = WaitingForNixConfig
	nixinitDirectory     = "/uploads/nixinit" // should not have trailing /
	configurationNixFile = "configuration.nix"
	nixosEtcDirectory    = "/etc/nixos"
)

//go:embed embed_files/flake.nix embed_files/hardware-configuration.nix embed_files/README.md
var nixFiles embed.FS

type responseParams struct {
	ServerVersion string
	ServerStatus  string
	Uptime        string
	LaunchTime    string
}

var responseTemplate = `
-----
nixinit-server version: {{ .ServerVersion }}
nixinit-server state: {{ .ServerStatus }}
uptime: {{ .Uptime }} (since {{ .LaunchTime }})
-----

Welcome to nixinit-server!

To complete initialiation of this server, you must upload an
appropriate nix configuration to the correct directory. The
directory is /uploads/nixinit/<instance-id>.

You can do this with the nixinit client or you can use scp
directly; see nixinit documentation for more information.

Terminating SSH session - goodbye!

`

func (n NixInitState) String() string {
	switch n {
	case WaitingForNixConfig:
		return "WAITING_FOR_NIX_CONFIG"
	case ConfiguringNixSystem:
		return "CONFIGURING_NIX_SYSTEM"
	case ShuttingDown:
		return "SHUTTING_DOWN"
	case NixinitError:
		return "NIXINIT_ERROR"
	default:
		return "UNKNOWN"
	}
}

func generateStandardResponse() (string, error) {
	launchTime := time.Now().Add(-24 * time.Hour) // Assume the server started 24 hours ago
	data := responseParams{
		ServerVersion: "0.0.1",
		ServerStatus:  currentState.String(),
		Uptime:        time.Since(launchTime).String(),
		LaunchTime:    launchTime.Format(time.RFC3339),
	}

	tmpl, err := template.New("response").Parse(responseTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	var result string
	builder := &strings.Builder{}
	err = tmpl.Execute(builder, data)
	if err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	result = builder.String()
	return result, nil
}

func sshSessionHandler(s ssh.Session) {
	pterm.Info.Println("new SSH session request")

	if s.User() != validUser {
		pterm.Info.Println("user invalid - terminating session...")
		_, err := s.Write([]byte("user invalid - closing ssh session...\n"))
		if err != nil {
			log.Printf("error writing to session: %v", err)
		}
		err = s.Exit(0)
		if err != nil {
			log.Printf("error exiting session: %v", err)
		}
		return
	}

	authorizedKey := gossh.MarshalAuthorizedKey(s.PublicKey())
	pterm.Info.Printf("log in attempt - user public key: %v\n", string(authorizedKey))

	standardResponse, err := generateStandardResponse()
	if err != nil {
		pterm.Error.Printf("error generating standard response: %v\n", err)
		_, err = s.Write([]byte(standardResponse))
		if err != nil {
			log.Printf("error writing to session: %v", err)
		}
		err = s.Exit(0)
		if err != nil {
			log.Printf("error exiting session: %v", err)
		}
		return
	}
	_, err = s.Write([]byte(standardResponse))
	if err != nil {
		log.Printf("error writing to session: %v", err)
	}
	err = s.Exit(0)
	if err != nil {
		log.Printf("error exiting session: %v", err)
	}
}

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	return true // allow all keys, or use ssh.KeysEqual() to compare against known keys
}

type fileGetHandler struct{}

func (f fileGetHandler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	path, filename := filepath.Split(r.Filepath)

	// Check if the path is under /uploads
	if !strings.HasPrefix(path, nixinitDirectory) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	transformedDirectory := addRootDirectory(sftpRootDirectory, path)
	transformedFilename := filepath.Join(transformedDirectory, filename)
	pterm.Info.Printf("file download request - path: %s, filename: %s\n", path, filename)

	// Check if the path is a directory
	info, err := os.Stat(transformedFilename)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	if info.IsDir() {
		// If it's a directory, return a listing
		files, err := os.ReadDir(transformedFilename)
		if err != nil {
			return nil, sftp.ErrSshFxFailure
		}

		var listing strings.Builder
		for _, file := range files {
			listing.WriteString(file.Name() + "\n")
		}

		return strings.NewReader(listing.String()), nil
	}

	// If it's not a directory, return the file content
	cleanedPath := filepath.Clean(transformedFilename)
	file, err := os.Open(cleanedPath)
	if err != nil {
		return nil, sftp.ErrSshFxNoSuchFile
	}

	return file, nil
}

type filePutHandler struct{}

func (f filePutHandler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	pterm.Info.Printf("file upload request...\n")
	path, filename := filepath.Split(r.Filepath)

	// Check if the path is under /uploads/nixinit
	if !strings.HasPrefix(r.Filepath, nixinitDirectory) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	transformedDirectory := addRootDirectory(sftpRootDirectory, path)
	transformedFilename := filepath.Join(transformedDirectory, filename)

	// Ensure the directory exists
	if err := os.MkdirAll(transformedDirectory, 0750); err != nil {
		return nil, sftp.ErrSshFxFailure
	}

	// Open the file for writing
	cleanedPath := filepath.Clean(transformedFilename)
	file, err := os.OpenFile(cleanedPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, sftp.ErrSshFxFailure
	}

	return file, nil
}

type fileCmdHandler struct{}

func (f fileCmdHandler) Filecmd(r *sftp.Request) error {
	pterm.Info.Printf("file command request - method: %s, path: %s, target: %s\n", r.Method, r.Filepath, r.Target)
	path := r.Filepath
	targetPath := r.Target

	// Check if the path is under /uploads/nixinit
	if !strings.HasPrefix(path, nixinitDirectory) {
		return sftp.ErrSshFxPermissionDenied
	}

	switch r.Method {
	case "Rename":
		// Check if the target path is also under /uploads/nixinit
		if !strings.HasPrefix(targetPath, nixinitDirectory) {
			return sftp.ErrSshFxPermissionDenied
		}

		err := os.Rename(path, targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return sftp.ErrSshFxNoSuchFile
			}
			return sftp.ErrSshFxFailure
		}

	case "Remove":
		err := os.Remove(path)
		if err != nil {
			if os.IsNotExist(err) {
				return sftp.ErrSshFxNoSuchFile
			}
			return sftp.ErrSshFxFailure
		}

	case "Mkdir":
		err := os.Mkdir(path, 0750)
		if err != nil {
			return sftp.ErrSshFxFailure
		}

	case "Rmdir":
		err := os.Remove(path) // os.Remove can also remove directories
		if err != nil {
			if os.IsNotExist(err) {
				return sftp.ErrSshFxNoSuchFile
			}
			return sftp.ErrSshFxFailure
		}

	case "Setstat":
		// For simplicity, we're not implementing file attribute changes
		// You could add implementation here if needed
		return sftp.ErrSshFxOpUnsupported

	default:
		return sftp.ErrSshFxOpUnsupported
	}

	return nil
}

type fileListHandler struct{}

func (f fileListHandler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	pterm.Info.Printf("file list request - path: %s\n", r.Filepath)
	path, filename := filepath.Split(r.Filepath)

	// Check if the path is under /uploads/nixinit
	if !strings.HasPrefix(path, nixinitDirectory) {
		return nil, sftp.ErrSshFxPermissionDenied
	}

	transformedDirectory := addRootDirectory(sftpRootDirectory, path)
	transformedFilename := filepath.Join(transformedDirectory, filename)

	// Check if the path exists
	info, err := os.Stat(transformedFilename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, sftp.ErrSshFxNoSuchFile
		}
		return nil, sftp.ErrSshFxFailure
	}

	if !info.IsDir() {
		// If it's not a directory, return info for the specific file
		return listerat([]os.FileInfo{info}), nil
	}

	// Read the directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, sftp.ErrSshFxFailure
	}

	// Convert os.DirEntry to os.FileInfo
	fileInfos := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, sftp.ErrSshFxFailure
		}
		fileInfos = append(fileInfos, info)
	}

	return listerat(fileInfos), nil
}

// listerat implements ListerAt interface
type listerat []os.FileInfo

func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n := copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

func sftpHandler(sess ssh.Session) {
	serverOptions := []sftp.RequestServerOption{}

	handlers := sftp.Handlers{
		FileGet:  fileGetHandler{},
		FilePut:  filePutHandler{},
		FileCmd:  fileCmdHandler{},
		FileList: fileListHandler{},
	}

	server := sftp.NewRequestServer(
		sess,
		handlers,
		serverOptions...,
	)
	if err := server.Serve(); err == io.EOF {
		err = server.Close()
		if err != nil {
			pterm.Warning.Printf("error closing sftp session: %v\n", err)
		} else {
			pterm.Info.Printf("sftp client exited session\n")
		}
	} else if err != nil {
		pterm.Info.Printf("sftp server completed with error: %v\n", err)
	}
}

func addRootDirectory(root, path string) string {
	return filepath.Join(root, strings.TrimPrefix(path, "/"))
}

func removeRootDirectory(root, path string) (string, error) {
	if !strings.HasPrefix(path, root) {
		return "", fmt.Errorf("path %q does not start with root %q", path, root)
	}
	pathWithSuffix := strings.TrimPrefix(path, root)
	if pathWithSuffix[0] != '/' {
		pathWithSuffix = "/" + pathWithSuffix
	}
	return strings.TrimSuffix(pathWithSuffix, "/"), nil
}

func watchDirectory(dirPath, instanceID string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					pterm.Info.Printf("File created or modified: %v\n", event.Name)

					// Handle other new files/directories
					handleNewFile(event.Name, instanceID)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("Error:", err)
			}
		}
	}()

	err = watcher.Add(dirPath)
	if err != nil {
		log.Printf("Error adding watcher to directory: %v\n", err)
		return fmt.Errorf("error adding watcher to directory: %v", err)
	}
	<-done
	return nil
}

// extractInstanceId extracts the instanceId from the directory path - the instanceId
// is the third directory in the path, ie /uploads/nixinit/<instance-id>/
func extractInstanceID(directory string) (string, error) {
	// ensure that directory does not have trailing /
	directory = strings.TrimSuffix(directory, "/")

	// remove /uploads/nixinit/ prefix
	directory = strings.TrimPrefix(directory, nixinitDirectory)

	directories := strings.Split(directory, "/")
	pterm.Info.Printf("directory: %v,directories: %v\n", directory, directories)

	if len(directories) > 0 {
		return directories[1], nil
	}
	return "", nil
}

func copyFile(src, dst string) error {
	cleanedSrc := filepath.Clean(src)
	sourceFile, err := os.Open(cleanedSrc)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create new file
	cleanedDst := filepath.Clean(dst)
	destFile, err := os.Create(cleanedDst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy the bytes to destination from source
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Sync to ensure writes are completed
	err = destFile.Sync()
	return err
}

func writeEmbeddedFile(fs embed.FS, srcPath, destPath string) error {
	data, err := fs.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded file %s: %v", srcPath, err)
	}

	err = os.WriteFile(destPath, data, 0600)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %v", destPath, err)
	}

	return nil
}

func generateConfigurationFiles(configurationPath string) error {
	// copy file from configurationPath to /etc/nixos/configuration.nix
	err := copyFile(configurationPath, filepath.Join(nixosEtcDirectory, "configuration.nix"))
	if err != nil {
		log.Printf("Error copying configuration file: %v\n", err)
		return err
	}

	// Write flake.nix
	if err := writeEmbeddedFile(nixFiles, "embed_files/flake.nix", filepath.Join(nixosEtcDirectory, "flake.nix")); err != nil {
		log.Printf("Error writing flake.nix: %v\n", err)
		return err
	}

	// Write hardware-configuration.nix
	if err := writeEmbeddedFile(nixFiles, "embed_files/hardware-configuration.nix", filepath.Join(nixosEtcDirectory, "hardware-configuration.nix")); err != nil {
		log.Printf("Error writing hardware-configuration.nix: %v\n", err)
		return err
	}

	// Write README.md
	if err := writeEmbeddedFile(nixFiles, "embed_files/README.md", filepath.Join(nixosEtcDirectory, "README.md")); err != nil {
		log.Printf("Error writing README.md: %v\n", err)
		return err
	}

	// write
	return nil
}

func runNixosRebuild(configurationPath string) error {
	log.Printf("Generating configuration files...\n")
	// TODO: remove reference to home/test-user
	err := generateConfigurationFiles(filepath.Join("/home/test-user", configurationPath))
	if err != nil {
		return fmt.Errorf("error generating configuration files: %v", err)
	}

	// create a new nixos generation
	// assume this is being run in privileged mode
	log.Printf("Running nixos-rebuild...\n")
	cmd := exec.Command("nixos-rebuild", "build")
	cmd.Dir = nixosEtcDirectory

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("Running nixos-rebuild...")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error running nixos build-vm: %v, stderr: %s", err, stderr.String())
	}

	log.Printf("nixos-rebuild complete: %s\n", out.String())
	return nil

}

func handleNewFile(filePath, instanceID string) {
	// Add your logic here to handle the new file
	// For example, you could process the file, move it, etc.
	pterm.Info.Printf("file watcher: New file uploaded: %s\n", filePath)

	// split the filename into the sftpRootDirectory, directory and filename
	transformedDirectory, filename := filepath.Split(filePath)
	directory, _ := removeRootDirectory(sftpRootDirectory, transformedDirectory)
	pterm.Info.Printf("file watcher: Directory: %s, Filename: %s\n", directory, filename)

	// directory should be /uploads/nixinit/<instance-id> - we need to check if this is the
	// same as the instanceId
	if strings.HasPrefix(directory, nixinitDirectory) {
		instanceIDInPath, _ := extractInstanceID(directory)
		pterm.Info.Printf("Instance ID in path: %s\n", instanceIDInPath)
		if instanceIDInPath == instanceID {
			pterm.Info.Printf("File uploaded to correct instance directory...%v\n", directory)
			if filename == configurationNixFile {
				pterm.Info.Printf("Configuration.nix file uploaded - starting nix reconfigure... \n")
				err := runNixosRebuild(filepath.Join(directory, filename))
				if err == nil {
					log.Printf("New nix configuration applied - rebooting in 30 seconds...\n")
				} else {
					log.Printf("Error applying new nix configuration - please upload a new configuration...\n")
				}
			}
		} else {
			pterm.Info.Printf("Instance directory does not match: %s\n", directory)
		}
	}
}

func startWatcher(sftpRootDirectory, path, file, instanceID string) {
	// we assume this directory already exists
	transformedDirectory := addRootDirectory(sftpRootDirectory, path)

	pterm.Info.Printf("Watching directory: %s\n", transformedDirectory)

	// Start watching the directory in a goroutine
	err := watchDirectory(transformedDirectory, instanceID)
	if err != nil {
		log.Fatalf("Failed to start watching directory: %v", err)
	}
}

func startShutdownHandler(timerDuration time.Duration) {
	pterm.Info.Printf("Starting shutdown handler with timer duration: %v - system will shut down at %v\n",
		timerDuration, time.Now().Add(timerDuration).Format(time.RFC3339))

	// Set up a timer to shut down the system after the specified duration
	timer := time.NewTimer(timerDuration)
	<-timer.C

	pterm.Info.Println("Shutdown handler triggered - system will shut down now")
}

func main() {

	instanceID, err := getInstanceID()
	if err != nil {
		log.Fatalf("Failed to get instance ID - continuing in unusable state%v", err)
		currentState = UnableToDetermineInstanceID
	}

	if sftpRootDirectory == "" {
		// set sftpRootDirectory to the current working directory
		sftpRootDirectory, err = os.Getwd()
		if err != nil {
			log.Fatalf("Failed to get current working directory: %v", err)
		}
		// make subdirectories /uploads/nixinit under the sftpRootDirectory
		nixinitUploadsDirectory := addRootDirectory(sftpRootDirectory, nixinitDirectory)
		instanceUploadsDirectory := filepath.Join(nixinitUploadsDirectory, instanceID)
		err = os.MkdirAll(instanceUploadsDirectory, 0750)
		if err != nil {
			log.Fatalf("Failed to create directory %s: %v", nixinitUploadsDirectory, err)
		}
	}

	timerDuration := time.Hour
	go startShutdownHandler(timerDuration)

	go startWatcher(sftpRootDirectory, filepath.Join(nixinitDirectory, instanceID), configurationNixFile, instanceID)

	serverEndpoint := fmt.Sprintf("%s:%d", host, port)

	server := ssh.Server{
		Addr:             serverEndpoint,
		Handler:          sshSessionHandler,
		PublicKeyHandler: publicKeyHandler,
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": sftpHandler,
		},
	}
	pterm.Info.Printf("Launching SSH server on %v\n", serverEndpoint)

	log.Fatal(server.ListenAndServe())
}
