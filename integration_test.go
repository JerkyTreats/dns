//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Set up the test environment
	cmd := exec.Command("docker-compose", "-f", "docker-compose.test.yml", "up", "-d", "--build")
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=0")

	// Capture output for debugging
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		// Print the actual error output for debugging
		panic("docker-compose up failed: " + err.Error() + "\nOutput: " + out.String())
	}

	// Give services time to start by polling the health endpoint
	apiURL := "http://localhost:8081/health"
	var healthy bool
	for i := 0; i < 30; i++ {
		resp, err := http.Get(apiURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			healthy = true
			resp.Body.Close()
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	if !healthy {
		panic("api service never became healthy")
	}

	// Run the tests
	code := m.Run()

	// Tear down the test environment
	cmd = exec.Command("docker-compose", "-f", "docker-compose.test.yml", "down")
	err = cmd.Run()
	if err != nil {
		// can't do much here
		panic("docker-compose down failed: " + err.Error())
	}
	os.RemoveAll("./ssl-test")

	os.Exit(code)
}

func TestAPI_AddRecord(t *testing.T) {
	// Let's create the zone file on the host, which is mounted into the container.
	zonesDir := "configs/coredns-test/zones"
	err := os.MkdirAll(zonesDir, 0755)
	require.NoError(t, err)

	zoneFilePath := zonesDir + "/test-service.zone"
	zoneContent := []byte("$ORIGIN test-service.test.jerkytreats.dev.\n@ IN SOA ns1.test.jerkytreats.dev. admin.test.jerkytreats.dev. 1 7200 3600 1209600 3600\n")

	// Create the file with world-readable permissions so the container can read it
	err = os.WriteFile(zoneFilePath, zoneContent, 0666)
	require.NoError(t, err)
	defer os.Remove(zoneFilePath) // Clean up the zone file

	// Now, add a record to that zone
	apiURL := "http://localhost:8081/add-record"
	requestBody, _ := json.Marshal(map[string]string{
		"service_name": "test-service",
		"name":         "www",
		"ip":           "192.168.10.1",
	})

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(requestBody))
	require.NoError(t, err)
	defer resp.Body.Close()

	// If we get an error, read the response body for debugging
	if resp.StatusCode != http.StatusCreated {
		body, _ := os.ReadFile(zoneFilePath)
		t.Logf("Zone file contents: %s", string(body))

		respBody := make([]byte, 1024)
		n, _ := resp.Body.Read(respBody)
		t.Logf("Response body: %s", string(respBody[:n]))
	}

	assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status created")
}

func TestDNS_QueryRecord(t *testing.T) {
	// Create a zone and a record
	zonesDir := "configs/coredns-test/zones"
	err := os.MkdirAll(zonesDir, 0755)
	require.NoError(t, err)

	zoneFilePath := zonesDir + "/test-query.zone"
	zoneContent := []byte("$ORIGIN test-query.test.jerkytreats.dev.\n@ IN SOA ns1.test.jerkytreats.dev. admin.test.jerkytreats.dev. 1 7200 3600 1209600 3600\nwww IN A 192.168.20.1\n")

	// Create the file with world-readable permissions
	err = os.WriteFile(zoneFilePath, zoneContent, 0666)
	require.NoError(t, err)
	defer os.Remove(zoneFilePath)

	// Add the zone to the Corefile
	corefilePath := "configs/coredns-test/Corefile"
	corefile, err := os.ReadFile(corefilePath)
	require.NoError(t, err)

	newCorefileContent := string(corefile) + "\ntest-query.test.jerkytreats.dev:53 {\n    file /etc/coredns/zones/test-query.zone\n    errors\n    log\n}\n"

	// Write with world-readable permissions
	err = os.WriteFile(corefilePath, []byte(newCorefileContent), 0666)
	require.NoError(t, err)
	defer os.WriteFile(corefilePath, corefile, 0666) // restore with same permissions

	// Reload CoreDNS - by restarting the container
	cmd := exec.Command("docker-compose", "-f", "docker-compose.test.yml", "restart", "coredns-test")
	err = cmd.Run()
	require.NoError(t, err, "docker-compose restart coredns-test failed")

	// Give CoreDNS more time to restart and load the new configuration
	time.Sleep(15 * time.Second)

	// Query the record using Docker network since localhost:5353 conflicts with macOS mDNS
	dnsCmd := exec.Command("docker", "run", "--rm", "--network", "dns_dns-test-network",
		"busybox", "nslookup", "www.test-query.test.jerkytreats.dev", "coredns-test")

	var out bytes.Buffer
	dnsCmd.Stdout = &out
	dnsCmd.Stderr = &out

	runErr := dnsCmd.Run()
	require.NoError(t, runErr, "nslookup command failed: %s", out.String())

	output := out.String()
	t.Logf("nslookup output: %s", output)

	// Check if the IP address is in the output
	assert.Contains(t, output, "192.168.20.1", "Expected IP not found in DNS response")
}
