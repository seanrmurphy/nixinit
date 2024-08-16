package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/perlogix/libdetectcloud"
)

// CloudProvider defines the different cloud providers that can be used -
// this is defined in libdetectcloud and can be any of the following:
// Amazon Web Services, Microsoft Azure, Digital Ocean
// Google Compute Engine, OpenStack, SoftLayer, Vultr
// K8S Container, Container
type CloudProvider int

const cidataVolumeName = "cidata"

// create enum for cloud providers
const (
	AWS CloudProvider = iota
	Azure
	DigitalOcean
	GCP
	OpenStack
	SoftLayer
	Vultr
	K8SContainer
	Container
)

var cloudProviderStrings = []string{
	"Amazon Web Services",
	"Microsoft Azure",
	"Digital Ocean",
	"Google Compute Engine",
	"OpenStack",
	"SoftLayer",
	"Vultr",
	"K8S Container",
	"Container",
}

// var cloudProviderToString = map[CloudProvider]string{
// 	AWS:          cloudProviderStrings[AWS],
// 	Azure:        cloudProviderStrings[Azure],
// 	DigitalOcean: cloudProviderStrings[DigitalOcean],
// 	GCP:          cloudProviderStrings[GCP],
// 	OpenStack:    cloudProviderStrings[OpenStack],
// 	SoftLayer:    cloudProviderStrings[SoftLayer],
// 	Vultr:        cloudProviderStrings[Vultr],
// 	K8SContainer: cloudProviderStrings[K8SContainer],
// 	Container:    cloudProviderStrings[Container],
// }

var stringToCloudProvider = map[string]CloudProvider{
	cloudProviderStrings[AWS]:          AWS,
	cloudProviderStrings[Azure]:        Azure,
	cloudProviderStrings[DigitalOcean]: DigitalOcean,
	cloudProviderStrings[GCP]:          GCP,
	cloudProviderStrings[OpenStack]:    OpenStack,
	cloudProviderStrings[SoftLayer]:    SoftLayer,
	cloudProviderStrings[Vultr]:        Vultr,
	cloudProviderStrings[K8SContainer]: K8SContainer,
	cloudProviderStrings[Container]:    Container,
}

func getAWSInstanceID() (string, error) {
	client := &http.Client{
		Timeout: time.Second * 5,
	}

	// Try IMDSv2 first
	token, err := getIMDSv2Token(client)
	if err == nil {
		instanceID, err := getInstanceIDWithToken(client, token)
		if err == nil {
			return instanceID, nil
		}
	}

	// Fallback to IMDSv1
	return getInstanceIDWithoutToken(client)
}

func getIMDSv2Token(client *http.Client) (string, error) {
	req, err := http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "21600")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting token, status code: %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(token), nil
}

func getInstanceIDWithToken(client *http.Client, token string) (string, error) {
	req, err := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/instance-id", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token", token)

	return getInstanceIDFromRequest(client, req)
}

func getInstanceIDWithoutToken(client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/instance-id", nil)
	if err != nil {
		return "", err
	}

	return getInstanceIDFromRequest(client, req)
}

func getInstanceIDFromRequest(client *http.Client, req *http.Request) (string, error) {
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting instance ID, status code: %d", resp.StatusCode)
	}

	instanceID, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(instanceID), nil
}

func getAzureInstanceID() (string, error) {
	return "", fmt.Errorf("Azure instance ID retrieval not supported")
}
func getDigitalOceanInstanceID() (string, error) {
	return "", fmt.Errorf("DigitalOcean instance ID retrieval not supported")
}
func getGCPInstanceID() (string, error) {
	return "", fmt.Errorf("GCP instance ID retrieval not supported")
}
func getOpenStackInstanceID() (string, error) {
	return "", fmt.Errorf("OpenStack instance ID retrieval not supported")
}
func getSoftLayerInstanceID() (string, error) {
	return "", fmt.Errorf("SoftLayer instance ID retrieval not supported")
}
func getVultrInstanceID() (string, error) {
	return "", fmt.Errorf("Vultr instance ID retrieval not supported")
}
func getK8SContainerInstanceID() (string, error) {
	return "", fmt.Errorf("K8s Container instance ID retrieval not supported")
}
func getContainerInstanceID() (string, error) {
	return "", fmt.Errorf("Container instance ID retrieval not supported")
}

func getInstanceIDWithCloud(cloud string) (string, error) {
	provider, ok := stringToCloudProvider[cloud]
	if !ok {
		return "", fmt.Errorf("unknown cloud provider: %s", cloud)
	}

	switch provider {
	case AWS:
		return getAWSInstanceID()
	case Azure:
		return getAzureInstanceID()
	case DigitalOcean:
		return getDigitalOceanInstanceID()
	case GCP:
		return getGCPInstanceID()
	case OpenStack:
		return getOpenStackInstanceID()
	case SoftLayer:
		return getSoftLayerInstanceID()
	case Vultr:
		return getVultrInstanceID()
	case K8SContainer:
		return getK8SContainerInstanceID()
	case Container:
		return getContainerInstanceID()
	default:
		return "", fmt.Errorf("unsupported cloud provider: %s", cloud)
	}
}

func isCidataVolumeAvailable() bool {
	labelPath := fmt.Sprintf("/dev/disk/by-label/%s", cidataVolumeName)
	_, err := os.Stat(labelPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		log.Printf("Error checking for cidata volume: %v", err)
		return false
	}
	return true
}

func findCidataMountPoint() (string, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("error opening /proc/mounts: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.Contains(fields[1], "cidata") {
			return fields[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading /proc/mounts: %v", err)
	}

	return "", fmt.Errorf("cidata volume mount point not found")
}

func isLabeledDeviceMounted(label string) (bool, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return false, fmt.Errorf("error opening /proc/mounts: %v", err)
	}
	defer file.Close()

	labelPath := fmt.Sprintf("/dev/disk/by-label/%s", label)
	// realPath, err := filepath.EvalSymlinks(labelPath)
	// if err != nil {
	// 	return false, fmt.Errorf("error resolving symlink: %v", err)
	// }

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == labelPath {
			return true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("error reading /proc/mounts: %v", err)
	}

	return false, nil
}

func mountBlockDevice(mountPoint, label string) error {
	device := fmt.Sprintf("/dev/disk/by-label/%s", label)

	// Create mount point if it doesn't exist
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("failed to create mount point: %v", err)
	}

	// Mount the device
	log.Printf("attempting to mount %s to %s\n", device, mountPoint)
	flags := syscall.MS_RDONLY | syscall.MS_NOATIME
	if err := syscall.Mount(device, mountPoint, "iso9660", uintptr(flags), ""); err != nil {
		return fmt.Errorf("failed to mount device: %v", err)
	}

	fmt.Printf("Successfully mounted %s to %s\n", device, mountPoint)
	return nil
}

func getInstanceIDFromCidataVolume(mountPoint string) (string, error) {
	// Check if the cidata volume is mounted
	log.Printf("checking if cidata volume is mounted")
	isMounted, err := isLabeledDeviceMounted(cidataVolumeName)
	if err != nil {
		log.Printf("Error checking if cidata volume is mounted: %v\n", err)
		return "", fmt.Errorf("error checking if cidata volume is mounted: %v", err)
	}

	if !isMounted {
		// If not mounted, attempt to mount it
		log.Printf("cidata volume is not mounting - attempting to mount...")
		err = mountBlockDevice(mountPoint, cidataVolumeName)
		if err != nil {
			log.Printf("Failed to mount cidata volume: %v\n", err)
			return "", fmt.Errorf("failed to mount cidata volume: %v", err)
		}
		log.Printf("Successfully mounted cidata volume to %s\n", mountPoint)
	} else {
		log.Printf("cidata volume is already mounted\n")
	}

	// Look for the meta-data file in the cidata volume
	metadataPath := filepath.Join(mountPoint, "meta-data")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", fmt.Errorf("failed to read meta-data file from cidata volume: %v", err)
	}

	// Parse the YAML content to find the instance_id
	lines := strings.Split(string(metadata), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "instance_id:") {
			instanceID := strings.TrimSpace(strings.TrimPrefix(line, "instance_id:"))
			return instanceID, nil
		}
	}

	return "", fmt.Errorf("instance_id not found in meta-data file")
}

func getInstanceID() (string, error) {

	// detectcloud.Detect() will return an empty string or
	// Amazon Web Services, Microsoft Azure, Digital Ocean
	// Google Compute Engine, OpenStack, SoftLayer, Vultr
	// K8S Container, Container

	cloud := libdetectcloud.Detect()

	if cloud != "" {
		log.Printf("Detected cloud provider: %s\n", cloud)
		instanceID, err := getInstanceIDWithCloud(cloud)
		if err != nil {
			log.Printf("Failed to get instance ID: %v\n", err)
			return "", fmt.Errorf("failed to get instance ID: %v", err)
		}
		return instanceID, nil
	}

	// we are either running on an unknown cloud or not on any cloud
	// we can't handle the case that it's an unknown cloud for now
	// (perhaps we could simply perform a http req on 169.254.169.254
	// and see what we get)
	// for now, we just check if there is an ID available in a mounted
	// vol

	if isCidataVolumeAvailable() {
		// If cidata volume is available, you might want to read an ID from it
		// For example:
		// return readIDFromCidataVolume()
		mountPoint := "/mnt/cidata"
		instanceID, err := getInstanceIDFromCidataVolume(mountPoint)
		return instanceID, err
	}
	return "", fmt.Errorf("Unable to retrieve instance ID - No cloud-init datasource found ")
}
