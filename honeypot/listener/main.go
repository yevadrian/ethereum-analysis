package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	device   = "any"
	httpPort = "8545"
)

var (
	trafficMap = make(map[string]*logEntry)
	mutex      = &sync.Mutex{}
)

type logEntry struct {
	Timestamp   time.Time              `bson:"timestamp"`
	Request     map[string]interface{} `bson:"request,omitempty"`
	Response    map[string]interface{} `bson:"response,omitempty"`
	Source      string                 `bson:"source"`
	Destination string                 `bson:"destination"`
}

func main() {
	clientOptions := options.Client().ApplyURI("mongodb://mongodb:mongodb@localhost:27017")
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer func() {
		if err = client.Disconnect(context.TODO()); err != nil {
			log.Fatalf("Failed to disconnect MongoDB: %v", err)
		}
	}()

	db := client.Database("ethereum")
	collectionName := "honeypot"

	err = createTimeSeriesCollection(db, collectionName)
	if err != nil {
		log.Fatalf("Failed to create time-series collection: %v", err)
	}

	collection := db.Collection(collectionName)

	handle, err := pcap.OpenLive(device, 65535, false, pcap.BlockForever)
	if err != nil {
		log.Fatalf("Error opening device %s: %v", device, err)
	}
	defer handle.Close()

	filter := fmt.Sprintf("tcp port %s", httpPort)
	if err := handle.SetBPFFilter(filter); err != nil {
		log.Fatalf("Error setting BPF filter: %v", err)
	}
	fmt.Printf("Listening on %s, filtering traffic on port %s...\n", device, httpPort)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	for packet := range packetSource.Packets() {
		processPacket(packet, collection)
	}
}

func createTimeSeriesCollection(db *mongo.Database, collectionName string) error {
	opts := options.CreateCollection().SetTimeSeriesOptions(
		options.TimeSeries().SetTimeField("timestamp"),
	)

	err := db.CreateCollection(context.TODO(), collectionName, opts)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return err
	}
	return nil
}

func processPacket(packet gopacket.Packet, collection *mongo.Collection) {
	networkLayer := packet.NetworkLayer()
	transportLayer := packet.TransportLayer()
	applicationLayer := packet.ApplicationLayer()

	if networkLayer == nil || transportLayer == nil || applicationLayer == nil {
		return
	}

	srcIP, dstIP := networkLayer.NetworkFlow().Endpoints()
	srcPort, dstPort := transportLayer.TransportFlow().Endpoints()

	payload := applicationLayer.Payload()

	isHTTP := dstPort.String() == httpPort || srcPort.String() == httpPort
	if !isHTTP {
		return
	}

	isRequest := dstPort.String() == httpPort
	body := cleanHTTPBody(payload)

	if len(body) == 0 || !isValidJSON(body) {
		return
	}

	jsonBody := parseJSON(body)

	key := fmt.Sprintf("%s:%s->%s:%s", srcIP, srcPort, dstIP, dstPort)
	if !isRequest {
		key = fmt.Sprintf("%s:%s->%s:%s", dstIP, dstPort, srcIP, srcPort)
	}

	mutex.Lock()
	defer mutex.Unlock()

	if isRequest {
		trafficMap[key] = &logEntry{
			Timestamp:   time.Now(),
			Request:     jsonBody,
			Source:      srcIP.String(),
			Destination: dstIP.String(),
		}
	} else {
		if entry, exists := trafficMap[key]; exists {
			entry.Response = jsonBody
			writeToMongoDB(entry, collection)
			delete(trafficMap, key)
		}
	}
}

func cleanHTTPBody(data []byte) []byte {
	separator := []byte("\r\n\r\n")
	parts := bytes.SplitN(data, separator, 2)
	if len(parts) < 2 {
		return nil
	}
	body := parts[1]

	if strings.Contains(string(data), "Transfer-Encoding: chunked") {
		return decodeChunkedBody(body)
	}

	return body
}

func decodeChunkedBody(data []byte) []byte {
	var decoded []byte
	reader := bytes.NewReader(data)

	for {
		var size int
		_, err := fmt.Fscanf(reader, "%x\r\n", &size)
		if err != nil || size == 0 {
			break
		}

		chunk := make([]byte, size)
		_, err = reader.Read(chunk)
		if err != nil {
			break
		}
		decoded = append(decoded, chunk...)

		reader.Read(make([]byte, 2))
	}

	return decoded
}

func isValidJSON(data []byte) bool {
	var js map[string]interface{}
	return json.Unmarshal(data, &js) == nil
}

func parseJSON(data []byte) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

func writeToMongoDB(entry *logEntry, collection *mongo.Collection) {
	_, err := collection.InsertOne(context.TODO(), entry)
	if err != nil {
		log.Printf("Failed to insert log entry into MongoDB: %v", err)
	}
}
