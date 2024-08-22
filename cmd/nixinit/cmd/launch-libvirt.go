package cmd

import (
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kdomanski/iso9660"

	"github.com/digitalocean/go-libvirt"
	"gopkg.in/yaml.v3"
)

var (
	defaultPoolName = "iso"
	isoFilename     = "/tmp/cidata.iso"
)

// UserData is the user-data section of the cloud-init configuration.
type UserData struct {
	Description string `yaml:"description,omitempty"`
}

// MetaData is the meta-data section of the cloud-init configuration.
type MetaData struct {
	InstanceID string `yaml:"instance_id,omitempty"`
}

func uploadBootstrapImage() {
	cloudflarePublicBucketURL := "https://pub-5e2d0f66ccb2405aa99e1cea5de9f473.r2.dev"
	imageName := "nixinit-bootstrap.qcow2"
	imageURL := cloudflarePublicBucketURL + "/" + imageName

	err := downloadAndUploadImage(imageURL, imageName)
	if err != nil {
		log.Fatalf("Failed to download and upload bootstrap image: %v", err)
	}
}

func getVMIPAddress(l *libvirt.Libvirt, dom libvirt.Domain) (string, error) {
	// Get the list of interface addresses for the domain
	ifaces, err := l.DomainInterfaceAddresses(dom, uint32(libvirt.DomainInterfaceAddressesSrcLease), 0)
	if err != nil {
		return "", fmt.Errorf("failed to get domain interface addresses: %v", err)
	}

	// Look for the first non-loopback IPv4 address
	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			// if addr.Type == libvirt.IPAddrTypeIPv4 && !strings.HasPrefix(addr.Addr, "127.") {
			// 	return addr.Addr, nil
			// }
			if addr.Type == int32(libvirt.IPAddrTypeIpv4) && !strings.HasPrefix(addr.Addr, "127.") {
				return addr.Addr, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable IP address found for the domain")
}

func downloadAndUploadImage(imageURL, imageName string) error {
	// // Download the image from Cloudflare
	// resp, err := http.Get(imageURL) // nolint
	// if err != nil {
	// 	return fmt.Errorf("failed to download image: %v", err)
	// }
	// defer resp.Body.Close()

	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download image: HTTP status %d", resp.StatusCode)
	}
	log.Printf("Downloaded %s", imageName)

	// Create a temporary file to store the downloaded image
	tempFile, err := os.CreateTemp("", imageName)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy the downloaded content to the temporary file
	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write image to temporary file: %v", err)
	}
	log.Printf("Wrote %s to temporary file", imageName)

	// get size of file in bytes
	fileInfo, err := os.Stat(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}
	fileSize := fileInfo.Size()

	// Connect to libvirt
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %v", err)
	}
	defer l.Disconnect()

	// Look up the default storage pool
	pool, err := l.StoragePoolLookupByName(defaultPoolName)
	if err != nil {
		return fmt.Errorf("failed to lookup storage pool: %v", err)
	}

	// Get the path of the storage pool
	_, _, _, _, err = l.StoragePoolGetInfo(pool)
	if err != nil {
		return fmt.Errorf("failed to get storage pool info: %v", err)
	}

	poolPath, err := l.StoragePoolGetXMLDesc(pool, 0)
	if err != nil {
		return fmt.Errorf("failed to get storage pool XML description: %v", err)
	}

	// Extract the path from the XML
	var poolXML struct {
		Path string `xml:"target>path"`
	}
	unmarshalErr := xml.Unmarshal([]byte(poolPath), &poolXML)
	if unmarshalErr != nil {
		return fmt.Errorf("failed to parse storage pool XML: %v", unmarshalErr)
	}
	// if err := xml.Unmarshal([]byte(poolPath), &poolXML); err != nil {
	// 	return fmt.Errorf("failed to parse storage pool XML: %v", err)
	// }

	// Construct the full path for the new image
	imagePath := filepath.Join(poolXML.Path, imageName)

	// Create a new volume in the storage pool
	volumeXML := fmt.Sprintf(`
<volume>
  <name>%s</name>
  <allocation>0</allocation>
  <capacity unit="bytes">%d</capacity>
  <target>
    <path>%s</path>
    <format type='raw'/>
  </target>
</volume>`, imageName, fileSize, imagePath)

	vol, err := l.StorageVolCreateXML(pool, volumeXML, 0)
	if err != nil {
		return fmt.Errorf("failed to create storage volume: %v", err)
	}

	// Open the temporary file for reading
	_, err = tempFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to seek to start of temporary file: %v", err)
	}
	fileInfo, err = tempFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	// Upload the image content to the new volume
	err = l.StorageVolUpload(vol, tempFile, 0, uint64(fileInfo.Size()), 0)
	if err != nil {
		return fmt.Errorf("failed to upload image content: %v", err)
	}

	log.Printf("Successfully downloaded and uploaded image %s to storage pool %s", imageName, defaultPoolName)
	return nil
}

func launchLibvirtInstance(qcowImageName, vmName string, memory uint64, vcpus uint) error {
	// create random instanceID
	instanceID := uuid.New().String()
	log.Printf("Generated instance ID: %v", instanceID)

	userData := UserData{Description: fmt.Sprintf("Created by nixinit for instance ID: %s", instanceID)}
	metaData := MetaData{InstanceID: instanceID}
	err := createISO(isoFilename, userData, metaData)
	if err != nil {
		return fmt.Errorf("failed to marshal user data and metadata: %v", err)
	}

	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
		return err
	}
	defer l.Disconnect()

	log.Printf("Connected to libvirt at %s", uri)

	// Check if the storage pool exists
	poolName := defaultPoolName
	pool, err := l.StoragePoolLookupByName(poolName)
	if err != nil {
		log.Printf("storage pool lookup failed: %v", err)
		return fmt.Errorf("storage pool lookup failed: %w", err)
	}

	// check if the image exists
	vol, err := l.StorageVolLookupByName(pool, qcowImageName)
	if err != nil {
		if libvirtErr, ok := err.(libvirt.Error); ok && libvirtErr.Code == uint32(libvirt.ErrNoStorageVol) {
			log.Printf("QCOW image  %s does not exist in pool %s - attempting to download from internet...", qcowImageName, poolName)
			uploadBootstrapImage()

		} else {
			log.Printf("storage volume lookup failed: %v", err)
			return fmt.Errorf("storage volume lookup failed: %w", err)
		}
	}
	log.Printf("Found QCOW image %s in pool %s", qcowImageName, poolName)

	// Get the path of the QCOW image
	qcowPath, err := l.StorageVolGetPath(vol)
	if err != nil {
		log.Printf("failed to get QCOW image path: %v", err)
		return fmt.Errorf("failed to get QCOW image path: %w", err)
	}

	// Create a new volume based on the QCOW image
	fileSize := 10 * 1024 * 1024 * 1024 // 10 GB
	newVolName := fmt.Sprintf("%s-%s", vmName, qcowImageName)
	newVolXML := fmt.Sprintf(`
    <volume>
      <name>%s</name>
      <allocation>0</allocation>
      <capacity unit="bytes">%d</capacity>
      <target>
        <format type='qcow2'/>
      </target>
      <backingStore>
        <path>%s</path>
        <format type='qcow2'/>
      </backingStore>
    </volume>`, newVolName, fileSize, qcowPath)

	newVol, err := l.StorageVolCreateXML(pool, newVolXML, 0)
	if err != nil {
		log.Printf("failed to create new volume: %v", err)
		return fmt.Errorf("failed to create new volume: %w", err)
	}

	// Get the path of the new volume
	newVolPath, err := l.StorageVolGetPath(newVol)
	if err != nil {
		log.Printf("failed to get new volume path: %v", err)
		return fmt.Errorf("failed to get new volume path: %w", err)
	}

	// Define the VM XML (use newVolPath instead of isoPath)
	xmlConfig := fmt.Sprintf(`
    <domain type='kvm'>
      <name>%s</name>
      <memory unit='MiB'>%d</memory>
      <vcpu>%d</vcpu>
		  <os>
				<type arch='x86_64' machine='pc-q35-8.2'>hvm</type>
				<boot dev='hd'/>
			</os>
      <devices>
        <disk type='file' device='disk'>
          <driver name='qemu' type='qcow2'/>
          <source file='%s'/>
          <target dev='vda' bus='virtio'/>
        </disk>
		    <disk type='file' device='cdrom'>
					<driver name='qemu' type='raw'/>
					<source file='%s'/>
					<target dev='vdb' bus='sata'/>
					<readonly/>
				</disk>
        <interface type='network'>
          <source network='default'/>
          <model type='virtio'/>
        </interface>
        <console type='pty'/>
      </devices>
    </domain>`, vmName, memory, vcpus, newVolPath, isoFilename)

	// Define the domain
	dom, err := l.DomainDefineXML(xmlConfig)
	if err != nil {
		log.Printf("failed to define domain - error: %v", err)
		log.Printf("it's possible that a domain with this name already exists, in which case you will need to remove the existing domain to bootstrap a new one...\n")
		return err
	}

	// Start the domain
	err = l.DomainCreate(dom)
	if err != nil {
		log.Printf("failed to start domain: %v", err)
		return fmt.Errorf("failed to start domain: %w", err)
	}

	// Get the status of the VM
	// TODO: - check that the VM is running before trying to get its IP address
	state, reason, err := l.DomainGetState(dom, 0)
	if err != nil {
		log.Printf("failed to get domain state: %v", err)
		return fmt.Errorf("failed to get domain state: %w", err)
	}
	if state != int32(libvirt.DomainRunning) {
		log.Printf("Domain state: %s (reason: %v)", getDomainStateString(state), reason)
		return fmt.Errorf("failed to start VM: domain state is not running")
	}

	// Print VM status
	log.Printf("Successfully launched VM '%s' from QCOW image '%s'\n", vmName, qcowImageName)
	log.Printf("Waiting for VM to boot...")

	// Try to get the IP address up to 3 times
	var ip string
	for i := 0; i < 3; i++ {
		log.Printf("Attempt %d to get VM IP address...", i+1)

		// Wait for 10 seconds before attempting to get the IP
		time.Sleep(10 * time.Second)

		ip, err = getVMIPAddress(l, dom)
		if err == nil {
			log.Printf("VM IP Address: %s (instance ID: %v)\n", ip, instanceID)
			break
		}

		log.Printf("Failed to get VM IP address: %v", err)

		if i == 2 {
			log.Printf("Failed to obtain VM IP address after 3 attempts")
			return fmt.Errorf("failed to obtain VM IP address after 3 attempts")
		}
	}

	return nil
}

// Add this helper function to convert state to string
func getDomainStateString(state int32) string {
	switch libvirt.DomainState(state) {
	case libvirt.DomainNostate:
		return "No State"
	case libvirt.DomainRunning:
		return "Running"
	case libvirt.DomainBlocked:
		return "Blocked"
	case libvirt.DomainPaused:
		return "Paused"
	case libvirt.DomainShutdown:
		return "Shutdown"
	case libvirt.DomainShutoff:
		return "Shutoff"
	case libvirt.DomainCrashed:
		return "Crashed"
	case libvirt.DomainPmsuspended:
		return "PM Suspended"
	default:
		return "Unknown"
	}
}

func createISO(filename string, userData UserData, metaData MetaData) error {
	writer, err := iso9660.NewWriter()
	if err != nil {
		log.Fatalf("failed to create writer: %s", err)
		return err
	}
	defer writer.Cleanup()

	userDataBytes, _ := yaml.Marshal(userData)
	err = writer.AddFile(bytes.NewReader(userDataBytes), "user-data")
	if err != nil {
		log.Printf("failed to add file: %s", err)
		return fmt.Errorf("failed to add file: %w", err)
	}

	metaDataBytes, _ := yaml.Marshal(metaData)
	err = writer.AddFile(bytes.NewReader(metaDataBytes), "meta-data")
	if err != nil {
		log.Printf("failed to add file: %s", err)
		return fmt.Errorf("failed to add file: %w", err)
	}

	cleanedFilename := filepath.Clean(filename)
	outputFile, err := os.OpenFile(cleanedFilename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		log.Printf("failed to create file: %s", err)
		return fmt.Errorf("failed to create file: %w", err)
	}

	err = writer.WriteTo(outputFile, "cidata")
	if err != nil {
		log.Printf("failed to write ISO image: %s", err)
		return fmt.Errorf("failed to write ISO image: %w", err)
	}

	err = outputFile.Close()
	if err != nil {
		log.Printf("failed to close output file: %s", err)
		return fmt.Errorf("failed to close output file: %w", err)
	}
	return nil
}

func getBootstrapVMs() ([]string, error) {

	var bootstrapVMs []string
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %v", err)
	}
	defer l.Disconnect()

	domains, _, err := l.ConnectListAllDomains(1, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %v", err)
	}

	for _, dom := range domains {
		// log.Printf("Domain ID: %v, Name: %v\n", dom.ID, dom.Name)
		if dom.Name == "nixinit" {
			bootstrapVMs = append(bootstrapVMs, fmt.Sprintf("%x", dom.UUID))
		}
	}

	return bootstrapVMs, nil
}

// ParseUUID converts a string UUID to a byte slice
func ParseUUID(uuid string) ([]byte, error) {
	uuid = strings.ReplaceAll(uuid, "-", "")
	return hex.DecodeString(uuid)
}

// getDomainName extracts the domain name from its XML description
// func getDomainName(l *libvirt.Libvirt, dom libvirt.Domain) (string, error) {
// 	xmlDesc, err := l.DomainGetXMLDesc(dom, 0)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to get domain XML description: %v", err)
// 	}
//
// 	type domainXML struct {
// 		Name string `xml:"name"`
// 	}
//
// 	var domain domainXML
// 	err = xml.Unmarshal([]byte(xmlDesc), &domain)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to unmarshal domain XML: %v", err)
// 	}
//
// 	return domain.Name, nil
// }

func removeInstance(instanceUUID string) error {
	// Parse the libvirt URI
	uri, _ := url.Parse(string(libvirt.QEMUSystem))

	// Connect to libvirt
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %v", err)
	}
	defer l.Disconnect()

	// Convert string UUID to byte array
	uuidBytes, err := ParseUUID(instanceUUID)
	if err != nil {
		return fmt.Errorf("failed to parse UUID: %v", err)
	}

	// Convert []byte to libvirt.UUID
	var libvirtUUID libvirt.UUID
	copy(libvirtUUID[:], uuidBytes)

	// Look up the domain by UUID
	dom, err := l.DomainLookupByUUID(libvirtUUID)
	if err != nil {
		return fmt.Errorf("failed to find domain with UUID %s: %v", instanceUUID, err)
	}

	// Check if the domain is running
	state, _, err := l.DomainGetState(dom, 0)
	if err != nil {
		return fmt.Errorf("failed to get domain state: %v", err)
	}

	// If the domain is running, destroy it
	if state == int32(libvirt.DomainRunning) {
		err = l.DomainDestroy(dom)
		if err != nil {
			return fmt.Errorf("failed to destroy running domain %s: %v", dom.Name, err)
		}
		log.Printf("Domain %s has been stopped\n", dom.Name)
	}

	// Undefine the domain
	err = l.DomainUndefineFlags(dom, libvirt.DomainUndefineKeepNvram)
	if err != nil {
		return fmt.Errorf("failed to undefine domain %s: %v", dom.Name, err)
	}

	log.Printf("Domain %s (UUID: %s) has been successfully removed\n", dom.Name, instanceUUID)
	return nil
}
