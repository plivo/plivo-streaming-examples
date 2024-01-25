package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Command line flags
var (
	port                         = flag.String("p", "8080", "port to listen")
	directoryPath                = flag.String("path", "", "directory path where the files will be stored. (default 'the current working directory')")
	isBidirectional              = flag.Bool("bidirectional", true, "bidirectional support")
	bidirectionalPayloadFilePath = flag.String("payload", "./payload.raw", "bidirectional payload file full path")
	timerInterval                = flag.Int("interval", 5, "Timer interval,  used for sending payload data at regular intervals.")
	maxTimes                     = flag.Int("max_times", 1, "How many times payload should be sent. This is used in conjunction with the timer to control the number of payload sends.")
	sampleRate                   = flag.Int("sample_rate", 8000, "payload sample rate")
)

// Other global variables
var (
	input_channel chan string
	addr          string
	cache         string
	homedir       string
)

const (
	EventTypeStart = "start"
	EventTypeMedia = "media"
	EventTypeStop  = "stop"
)

// Structures for handling different types of events
type MediaEvent struct {
	Track     string `json:"track"`
	Timestamp string `json:"timestamp"`
	Payload   string `json:"payload"`
	Chunk     int    `json:"chunk"`
}

type StopEvent struct {
	CallId   string   `json:"callId"`
	StreamId string   `json:"streamId"`
	Tracks   []string `json:"tracks"`
}

type StartEvent struct {
	CallId   string   `json:"callId"`
	StreamId string   `json:"streamId"`
	Tracks   []string `json:"tracks"`
}

type Events struct {
	Event    string     `json:"event"`
	Start    StartEvent `json:"start"`
	Media    MediaEvent `json:"media"`
	Stop     StopEvent  `json:"stop"`
	StreamId string     `json:"streamId"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{} // use default options

// Structure for sending audio data
type SendAudio struct {
	Event string    `json:"event"`
	Media AudioData `json:"media"`
}

// Structure for audio data
type AudioData struct {
	ContentType string `json:"contentType"`
	Payload     string `json:"payload"`
	SampleRate  int    `json:"sampleRate"`
}

// buildPayload reads the content of the file with the given name,
// encodes it in base64, and constructs a JSON payload with the encoded data.
// The constructed payload is cached for future use.
// It returns the constructed payload as a string.
func buildPayload(fname string) string {
	if len(cache) > 0 {
		log.Println("Using cache..")
		return cache
	}

	f, err := os.Open(fname)
	if err != nil {
		fmt.Println("Unable to open file ", err)
		return ""
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		fmt.Println("Unable to read file", err)
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(content)

	// Construct the JSON payload (stream-in)
	data := SendAudio{
		Event: "playAudio",
		Media: AudioData{
			ContentType: "raw",
			Payload:     encoded,
			SampleRate:  *sampleRate,
		},
	}

	payloadJson, err := json.Marshal(data)
	if err != nil {
		log.Println("Unable to marshal JSON:", err)
		return ""
	}

	log.Println("====== payload details ============")
	log.Println(fname, len(payloadJson))
	log.Println("==================================")
	cache = string(payloadJson)
	return cache
}

// Structure for handling a stream
type StreamHandling struct {
	stream_id  string
	raw_fp     *os.File
	ib_fp      *os.File
	ob_fp      *os.File
	sent_count int
}

// Function to close files associated with a stream
func (s *StreamHandling) Close() {
	if s.raw_fp != nil {
		s.raw_fp.Close()
	}
	if s.ib_fp != nil {
		s.ib_fp.Close()
	}
	if s.ob_fp != nil {
		s.ob_fp.Close()
	}
	log.Print("All files related to ", s.stream_id, " closed")
}

// Function to write inbound track data file
func (s *StreamHandling) writeToInboundFile(data string) {
	if _, err := s.ib_fp.WriteString(data); err != nil {
		log.Println("Writing to inbound file failed.", err)
	}
}

// Function to write outbound track data file
func (s *StreamHandling) writeToOutboundFile(data string) {
	if _, err := s.ob_fp.WriteString(data); err != nil {
		log.Println("Writing to outbound file failed.", err)
	}
}

// Function to write data to the raw file (raw data from websocket connection)
func (s *StreamHandling) writeToRawFile(data string) {
	if _, err := s.raw_fp.WriteString(data); err != nil {
		log.Println("Writing to raw data file failed.", err)
	}
}

// Function to open files associated with a stream
func (s *StreamHandling) openFiles(directory string) {
	var err error
	rawFilePath := filepath.Join(directory, "raw.json")
	inboundFilePath := filepath.Join(directory, "inbound.raw")
	outbondFilePath := filepath.Join(directory, "outbound.raw")

	if s.raw_fp, err = os.OpenFile(rawFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		log.Print("Unable to open file", rawFilePath, err)
		return
	}
	if s.ib_fp, err = os.OpenFile(inboundFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		log.Print("Unable to open file", inboundFilePath, err)
		return
	}
	if s.ob_fp, err = os.OpenFile(outbondFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		log.Print("Unable to open file", outbondFilePath, err)
		return
	}
}

// Function to handle different events
func (s *StreamHandling) eventHandler(d string, e Events) {
	switch e.Event {
	case EventTypeStart:
		// log.Print("received start event")
		log.Print(e.Start.StreamId)
		os.Mkdir(e.Start.StreamId, 0777)
		s.stream_id = e.Start.StreamId
		s.openFiles(filepath.Join(homedir, s.stream_id))
	case EventTypeMedia:
		// log.Print("received media event")
		if s.ib_fp == nil && s.ob_fp == nil {
			log.Print("Media received without start!")
			if _, err := os.Stat(e.StreamId); os.IsNotExist(err) {
				fmt.Printf("Folder %s does not exist, error: %v", e.StreamId, err)
				return
			}
			s.openFiles(filepath.Join(homedir, s.stream_id))
		}
		rawDecodedData, err := base64.StdEncoding.DecodeString(e.Media.Payload)
		if err != nil {
			log.Println("Error decoding base64 payload:", err)
			return
		}
		if e.Media.Track == "inbound" {
			s.writeToInboundFile(string(rawDecodedData))
		} else {
			s.writeToOutboundFile(string(rawDecodedData))
		}
	case EventTypeStop:
		log.Print("received stop event")
	default:
		log.Print("received unknown event")
		return
	}
	s.writeToRawFile(d)
}

// WebSocket handler function
func handler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("Cannot upgrade:", err)
		return
	}
	defer c.Close()

	// Create a ticker to send data from file on the WebSocket connection at regular intervals
	ticker := time.NewTicker(time.Duration(*timerInterval) * time.Second)

	// to handle stream-out data.
	streamData := StreamHandling{stream_id: "", ob_fp: nil, ib_fp: nil, raw_fp: nil, sent_count: 0}
	defer streamData.Close()

	for {
		event := Events{}
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		json.Unmarshal(message, &event)
		if err != nil {
			log.Println("Error unmarshaling JSON:", err)
			break
		}

		// process data from the websocket connection. (stream-out)
		streamData.eventHandler(string(message), event)

		// Send data from file on the WebSocket connection at regular intervals (stream-in)
		if *isBidirectional == true {
			select {
			case <-ticker.C:
				payload := buildPayload(*bidirectionalPayloadFilePath)

				// ---- SENDING Stream-in Data ----
				log.Println("Sending payload.. ")
				c.WriteMessage(1, []byte(payload))

				streamData.sent_count = streamData.sent_count + 1
				if streamData.sent_count >= *maxTimes {
					ticker.Stop()
					break
				}
			default:
			}
		}
	}
	log.Printf("Connection closed")
}

func main() {
	var err error
	flag.Parse()
	fmt.Println("Running on ", *port)

	addr = strings.Join([]string{":", *port}, "")

	if *directoryPath != "" {
		homedir = *directoryPath
	} else {
		homedir, err = os.Getwd()
		if err != nil {
			log.Print("unable to get cwd.. exiting")
			panic(0)
		}
	}
	log.Print(homedir)

	log.SetFlags(log.Lmicroseconds | log.LstdFlags | log.Lshortfile)

	http.HandleFunc("/ws/", handler)

	log.Fatal(http.ListenAndServe(addr, nil))
}
