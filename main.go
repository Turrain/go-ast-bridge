package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/CyCoreSystems/audiosocket"
	"github.com/JexSrs/go-ollama"
	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
	"github.com/maxhawkins/go-webrtcvad"
	"github.com/pkg/errors"
)

const slinChunkSize = 320 // 8000Hz * 20ms * 2 bytes
const websocketURI = "ws://localhost:8011/ws"
const listenAddr = ":9092"

var API = NewChatAPI("http://127.0.0.1:8009/api")
var LLM = ollama.New(url.URL{Scheme: "http", Host: "localhost:11434"})
var ErrHangup = errors.New("Hangup")

func main() {
	var err error
	ctx := context.Background()
	log.Println("listening for AudioSocket connections on", listenAddr)
	if err = Listen(ctx); err != nil {
		log.Fatalln("listen failure:", err)
	}

	log.Println("exiting")
}
func Listen(ctx context.Context) error {
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return errors.Wrapf(err, "failed to bind listener to socket %s", listenAddr)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println("failed to accept new connection:", err)
			continue
		}

		go Handle(ctx, conn)
	}
}
func getCallID(c net.Conn) (uuid.UUID, error) {
	m, err := audiosocket.NextMessage(c)
	if err != nil {
		return uuid.Nil, err
	}
	if m.Kind() != audiosocket.KindID {
		return uuid.Nil, errors.Errorf("invalid message type %d getting CallID", m.Kind())
	}
	return uuid.FromBytes(m.Payload())
}

func Handle(pCtx context.Context, c net.Conn) {
	ctx, cancel := context.WithCancel(pCtx)
	defer cancel()
	defer c.Close()
	vad, err := webrtcvad.New()
	if err != nil {
		log.Fatal(err)
	}
	if err := vad.SetMode(3); err != nil {
		log.Fatal(err)
	}
	id, err := getCallID(c)
	if err != nil {
		log.Println("failed to get call ID:", err)
		return
	}
	log.Printf("processing call %s", id.String())

	ChatID := id.String()
	chat, err := API.GetChat(ChatID)

	if err != nil {
		log.Println("chat session doesn't exist, creating new one")
		_, err = API.StartChat(ChatID)
		if err != nil {
			log.Println("failed to start chat session:", err)
			return
		}
	}
	sttSettings, err := API.GetSttSettings(ChatID)
	if err != nil {
		log.Println("failed to get stt settings:", err)
		return
	}
	log.Println("STT Settings:", *sttSettings)
	llmSettings, err := API.GetLlmSettings(ChatID)
	if err != nil {
		log.Println("failed to get llm settings:", err)
		return
	}
	log.Println("LLM Settings:", *llmSettings)

	llmChat := LLM.GetChat(ChatID)

	if llmChat == nil {
		log.Println("creating new llm chat")
		_, err := LLM.Chat(&ChatID, LLM.Chat.WithModel("gemma2:9b"))
		if err != nil {
			log.Println("failed to get chat:", err)
			return
		}
	}
	llmChat = LLM.GetChat(ChatID)
	llmChat.DeleteAllMessages()
	log.Println("LLM Settings:", *llmSettings)
	messages := chat.Messages

	systemRole := "system"
	systemMessage := ollama.Message{
		Role:    &systemRole,
		Content: llmSettings.SystemPrompt,
	}
	llmChat.AddMessage(systemMessage)
	for _, message := range messages {
		message := ollama.Message{
			Role:    &message.Role,
			Content: &message.Content,
		}
		llmChat.AddMessage(message)
	}

	rate := 16000
	silenceThreshold := 5
	var inputAudioBuffer [][]float32
	var silenceCount int

	for ctx.Err() == nil {
		m, err := audiosocket.NextMessage(c)
		if errors.Cause(err) == io.EOF {
			log.Println("audiosocket closed")
			return
		}

		switch m.Kind() {
		case audiosocket.KindHangup:
			log.Println("audiosocket received hangup command")
			return
		case audiosocket.KindError:
			log.Println("error from audiosocket")
		case audiosocket.KindSlin:
			if m.ContentLength() < 1 {
				log.Println("no audio data")
				continue
			}
			audioData := m.Payload()
			//	threshold := int16(0x02)
			//	audioDataReduced := NoiseGate(audioData, threshold)
			floatArray, err := pcmToFloat32Array(audioData)
			if err != nil {
				log.Println("error converting pcm to float32:", err)
				continue
			}

			if active, err := vad.Process(rate, audioData); err != nil {
				log.Println("Error processing VAD:", err)
			} else if active {
				inputAudioBuffer = append(inputAudioBuffer, floatArray)
				calculateAudioLength(inputAudioBuffer, rate)
				silenceCount = 0
			} else {
				silenceCount++
				if silenceCount > silenceThreshold {
					if len(inputAudioBuffer) > 0 {
						log.Println("Processing complete sentence")
						handleInputAudio(c, inputAudioBuffer, ChatID, *sttSettings, *llmSettings)
						inputAudioBuffer = nil // Reset buffer
					}
				}
			}

		}
	}
}
func handleInputAudio(conn net.Conn, buffer [][]float32, chatID string, sttSettings STTSettings, llmSettings LLMSettings) {
	// Merge and process buffer, then send to server
	var mergedBuffer []float32
	for _, data := range buffer {
		mergedBuffer = append(mergedBuffer, data...)
	}
	length := calculateAudioLength(buffer, 16000)
	log.Println("Audio length:", length)
	if length < 0.40 {
		log.Println("Audio length is less than 0.45 seconds, skipping processing.")
		return
	}
	sttSettings.Language = "ru"
	transcription, err := sendFloat32ArrayToServer("http://localhost:8002/complete_transcribe_r", mergedBuffer, sttSettings)
	if err != nil {
		log.Println("Error sending data to server:", err)
		return
	}

	llmOptions := ollama.Options{
		Seed:          llmSettings.Seed,
		Mirostat:      llmSettings.Mirostat,
		MirostatEta:   llmSettings.MirostatEta,
		MirostatTau:   llmSettings.MirostatTau,
		NumCtx:        llmSettings.NumCtx,
		RepeatLastN:   llmSettings.RepeatLastN,
		RepeatPenalty: llmSettings.RepeatPenalty,
		TfsZ:          llmSettings.TfsZ,
		Temperature:   llmSettings.Temperature,

		NumPredict: llmSettings.NumPredict,
	}
	log.Println("LLM Option - Seed:", llmSettings.Seed)
	log.Println("LLM Option - Mirostat:", llmSettings.Mirostat)
	log.Println("LLM Option - MirostatEta:", llmSettings.MirostatEta)
	log.Println("LLM Option - MirostatTau:", llmSettings.MirostatTau)
	log.Println("LLM Option - NumCtx:", llmSettings.NumCtx)
	log.Println("LLM Option - RepeatLastN:", llmSettings.RepeatLastN)
	log.Println("LLM Option - RepeatPenalty:", llmSettings.RepeatPenalty)
	log.Println("LLM Option - TfsZ:", llmSettings.TfsZ)
	log.Println("LLM Option - NumPredict:", llmSettings.NumPredict)
	log.Println("LLM Option - Temperature:", llmSettings.Temperature)

	log.Println("LLM Options:", llmOptions)
	excludedWords := []string{"Продолжение следует...", "Субтитры сделал DimaTorzok", "Субтитры создавал DimaTorzok"}
	for _, word := range excludedWords {
		if transcription == word {
			log.Println("Transcription contains excluded word, stopping further processing.")
			return
		}
	}
	_, err = API.SendMessage(chatID, "user", transcription)
	if err != nil {
		log.Println("Error sending user message:", err)
		return
	}
	log.Println("Sent user message:", transcription)

	res, err := LLM.Chat(
		&chatID,
		LLM.Chat.WithModel("gemma2:9b"),

		LLM.Chat.WithMessage(ollama.Message{
			Role:    &UserSender,
			Content: &transcription,
		}),
		LLM.Chat.WithOptions(llmOptions),
	)
	if err != nil {
		log.Println("Error generating Ollama chat:", err)
		return
	}
	log.Println("Received result:", *res.Message.Content)
	_, err = API.SendMessage(chatID, "assistant", *res.Message.Content)
	if err != nil {
		log.Println("Error sending assistant message:", err)
		return
	}

	data := map[string]interface{}{
		"message":    *res.Message.Content,
		"language":   "ru",
		"speed":      1.0,
		"await_time": 0.015,
	}
	log.Println("Using transcription:", transcription)

	websocketSendReceive(websocketURI, data, conn)

}

func calculateAudioLength(inputAudioBuffer [][]float32, sampleRate int) float64 {
	// Calculate total number of samples in the buffer
	totalSamples := 0
	for _, buffer := range inputAudioBuffer {
		totalSamples += len(buffer)
	}

	// Calculate length in seconds
	lengthInSeconds := float64(totalSamples) / float64(sampleRate)
	return lengthInSeconds
}

func NoiseGate(input []byte, threshold int16) []byte {
	sampleCount := len(input) / 2
	output := make([]byte, len(input))

	for i := 0; i < sampleCount; i++ {
		// Extract the sample (int16) from the byte slice
		sample := int16(binary.LittleEndian.Uint16(input[i*2 : i*2+2]))

		// Apply the noise gate
		if math.Abs(float64(sample)) < float64(threshold) {
			sample = 0
		}

		// Store the processed sample back as bytes
		binary.LittleEndian.PutUint16(output[i*2:i*2+2], uint16(sample))
	}

	return output
}

func pcmToFloat32Array(pcmData []byte) ([]float32, error) {
	if len(pcmData)%2 != 0 {
		return nil, fmt.Errorf("pcm data length must be even")
	}

	float32Array := make([]float32, len(pcmData)/2)
	buf := bytes.NewReader(pcmData)

	for i := 0; i < len(float32Array); i++ {
		var sample int16
		if err := binary.Read(buf, binary.LittleEndian, &sample); err != nil {
			return nil, fmt.Errorf("failed to read sample: %v", err)
		}
		float32Array[i] = float32(sample) / 32768.0
	}

	return float32Array, nil
}
func sendFloat32ArrayToServer(serverAddress string, float32Array []float32, settings STTSettings) (string, error) {
	// Buffer to store the audio data
	var audioBuffer bytes.Buffer
	for _, f := range float32Array {
		if err := binary.Write(&audioBuffer, binary.LittleEndian, f); err != nil {
			return "", fmt.Errorf("failed to write float32: %v", err)
		}
	}

	// Create a new multipart writer
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add audio data to the multipart form
	audioWriter, err := writer.CreateFormFile("audio", "audio.raw")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}
	if _, err = io.Copy(audioWriter, &audioBuffer); err != nil {
		return "", fmt.Errorf("failed to copy audio data: %v", err)
	}

	// Add settings as a JSON string to the multipart form
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return "", fmt.Errorf("error marshalling settings JSON: %v", err)
	}

	if err = writer.WriteField("settings", string(settingsJSON)); err != nil {
		return "", fmt.Errorf("failed to write settings field: %v", err)
	}

	// Close the writer
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %v", err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", serverAddress, &requestBody)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	// Read and parse the response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("error unmarshalling response body: %v", err)
	}

	// Extract the transcription from the result
	if transcription, ok := result["transcription"].(string); ok {
		log.Println("Transcription:", transcription)
		return transcription, nil
	} else {
		return "", fmt.Errorf("transcription not found in response")
	}
}

var wsMutex sync.Mutex
var wsCancel context.CancelFunc

func websocketSendReceive(uri string, data map[string]interface{}, conn net.Conn) {
	wsMutex.Lock()
	if wsCancel != nil {
		wsCancel() // Cancel the previous instance
	}
	ctx, cancel := context.WithCancel(context.Background())
	wsCancel = cancel
	wsMutex.Unlock()

	go func() {
		defer func() {
			wsMutex.Lock()
			wsCancel = nil
			wsMutex.Unlock()
		}()

		wsConn, _, err := websocket.DefaultDialer.Dial(uri, nil)
		audioWriter := &AudioWriter{conn: conn}
		if err != nil {
			log.Println("Failed to connect to WebSocket:", err)
			return
		}
		defer wsConn.Close()

		err = wsConn.WriteJSON(data)
		if err != nil {
			log.Println("Failed to send JSON:", err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				log.Println("WebSocket connection closed by new instance")
				return
			default:
				messageType, message, err := wsConn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("Unexpected WebSocket closure: %v", err)
					}
					return
				}

				switch messageType {
				case websocket.TextMessage:
					var jsonMessage map[string]interface{}
					if err := json.Unmarshal(message, &jsonMessage); err == nil {
						if typeField, ok := jsonMessage["type"].(string); ok && typeField == "end_of_audio" {
							log.Println("End of conversation")
							return
						}
						log.Println("Received message:", jsonMessage)
					} else {
						log.Println("Failed to unmarshal JSON message:", err)
					}
				case websocket.BinaryMessage:
					log.Println("Received binary message:")
					if _, err := audioWriter.Write(message); err != nil {
						log.Println("Error writing to connection:", err)
						return
					}
				default:
					log.Printf("Received unsupported message type: %v", messageType)
				}
			}
		}
	}()
}

type AudioWriter struct {
	mutex sync.Mutex
	conn  net.Conn
}

func (aw *AudioWriter) Write(p []byte) (n int, err error) {
	aw.mutex.Lock()
	defer aw.mutex.Unlock()

	n, err = aw.conn.Write(audiosocket.SlinMessage(p))
	if err != nil {
		return n, err
	}

	return n, nil
}

// func noiseGate(samples []float64, threshold float64) []float64 {
// 	for i, sample := range samples {
// 		if math.Abs(sample) < threshold {
// 			samples[i] = 0
// 		}
// 	}
// 	return samples
// }
