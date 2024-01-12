// Example program: Use a Hermes connection to ping a vehicle

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lotharbach/tesla-hermes-signaling/hermes"
	log "github.com/sirupsen/logrus"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/vehicle"
)

func main() {
	status := 1
	debug := false
	defer func() {
		os.Exit(status)
	}()

	// Provided through command line options
	var (
		privateKeyFile    string
		vin               string
		ownerApiTokenFile string
	)
	flag.StringVar(&privateKeyFile, "key", "", "Private key `file` for authorizing commands (PEM PKCS8 NIST-P256)")
	flag.StringVar(&vin, "vin", "", "Vehicle Identification Number (`VIN`) of the car")
	flag.StringVar(&ownerApiTokenFile, "ownerApiToken", "", "Owner API access token in a file")
	flag.BoolVar(&debug, "debug", false, "Enable debugging")
	flag.Parse()

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if vin == "" {
		log.Printf("Must specify VIN")
		return
	}

	var err error

	ownerApiToken, err := os.ReadFile(ownerApiTokenFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading token from file: %s\n", err)
		return
	}

	var privateKey protocol.ECDHPrivateKey
	if privateKeyFile != "" {
		if privateKey, err = protocol.LoadPrivateKey(privateKeyFile); err != nil {
			log.Printf("Failed to load private key: %s", err)
			return
		}
	}

	userToken, err := fetchHermesToken(ctx, "users/jwt/hermes", string(ownerApiToken))
	if err != nil {
		log.Printf("Failed to fetch user hermes tokens from owner API: %s", err)
		return
	}

	vehicleToken, err := fetchHermesToken(ctx, "vehicles/"+vin+"/jwt/hermes", string(ownerApiToken))
	if err != nil {
		log.Printf("Failed to fetch vehicle hermes tokens from owner API: %s", err)
		return
	}

	conn, err := hermes.NewConnection(vin, userToken, vehicleToken)
	if err != nil {
		log.Printf("Failed to connect to vehicle: %s", err)
		return
	}
	defer conn.Close()

	car, err := vehicle.NewVehicle(conn, privateKey, nil)
	if err != nil {
		log.Printf("Failed to connect to vehicle: %s", err)
		return
	}

	if err := car.Connect(ctx); err != nil {
		log.Printf("Failed to connect to vehicle: %s\n", err)
		return
	}
	defer car.Disconnect()

	// Most interactions with the car require an authenticated client.
	// StartSession() performs a handshake with the vehicle that allows
	// subsequent commands to be authenticated.
	if err := car.StartSession(ctx, nil); err != nil {
		log.Printf("Failed to perform handshake with vehicle: %s\n", err)
		return
	}

	fmt.Println("Pinging car...")
	if err := car.Ping(ctx); err != nil {
		log.Printf("Failed to ping: %s\n", err)
		return
	}
	fmt.Println("Success!")
	status = 0
}

func fetchHermesToken(ctx context.Context, path, ownerApiToken string) (string, error) {
	ownerApiURL := "https://owner-api.teslamotors.com/api/1/"

	uuid := fmt.Sprintf("{\"uuid\": \"%s\" }", uuid.New())
	req, _ := http.NewRequest("POST", ownerApiURL+path, bytes.NewBufferString(uuid))

	req.Header.Add("Authorization", "Bearer "+strings.TrimSpace(ownerApiToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TeslaHermesExperiment")

	client := &http.Client{}
	resp, err := client.Do(req)

	var response struct {
		Token string `json:"token"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", err
	}
	return response.Token, nil
}
