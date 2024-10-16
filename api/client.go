package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

// Sender represents the role of the message sender.
type Sender string

const (
	SenderUser      Sender = "user"
	SenderAssistant Sender = "assistant"
	SenderSystem    Sender = "system"
)

// User represents a user in the system.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	// Add other relevant fields
}

// Chat represents a chat session.
type Chat struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Messages []Message `json:"messages"`
	Settings Settings  `json:"settings"`
	// Add other relevant fields
}

// Message represents a single message in a chat.
type Message struct {
	ID      int    `json:"id"`
	ChatID  string `json:"chatId"`
	Role    Sender `json:"role"`
	Content string `json:"content"`
	// Add other relevant fields
}

// Settings represents the settings for a chat.
type Settings struct {
	STTSettings      STTSettings      `json:"sttSettings"`
	LLMSettings      LLMSettings      `json:"llmSettings"`
	TTSSettings      TTSSettings      `json:"ttsSettings"`
	AsteriskSettings AsteriskSettings `json:"asteriskSettings"`
}

// STTSettings represents the settings for speech-to-text.
type STTSettings struct {
	Language                      *string  `json:"language"`
	BeamSize                      *int     `json:"beam_size"`
	BestOf                        *int     `json:"best_of"`
	Patience                      *int     `json:"patience"`
	NoSpeechThreshold             *int     `json:"no_speech_threshold"`
	Temperature                   *float64 `json:"temperature"`
	HallucinationSilenceThreshold *int     `json:"hallucination_silence_threshold"`
}

// LLMSettings represents the settings for the language model.
type LLMSettings struct {
	Seed          *int     `json:"seed"`
	Model         *string  `json:"model"`
	SystemPrompt  *string  `json:"system_prompt"`
	Mirostat      *int     `json:"mirostat"`
	MirostatEta   *float64 `json:"mirostat_eta"`
	MirostatTau   *float64 `json:"mirostat_tau"`
	NumCtx        *int     `json:"num_ctx"`
	RepeatLastN   *int     `json:"repeat_last_n"`
	RepeatPenalty *float64 `json:"repeat_penalty"`
	Temperature   *float64 `json:"temperature"`
	TfsZ          *float64 `json:"tfs_z"`
	NumPredict    *int     `json:"num_predict"`
	TopK          *int     `json:"top_k"`
	TopP          *float64 `json:"top_p"`
	MinP          *float64 `json:"min_p"`
}

// TTSSettings represents the settings for text-to-speech.
type TTSSettings struct {
	Voice string   `json:"voice"`
	Speed *float64 `json:"speed"`
}

// AsteriskSettings represents the settings for Asterisk.
type AsteriskSettings struct {
	AsteriskMinAudioLength   *int   `json:"asterisk_min_audio_length"`
	AsteriskSilenceThreshold *int   `json:"asterisk_silence_threshold"`
	AsteriskHost             string `json:"asterisk_host"`
	AsteriskNumber           string `json:"asterisk_number"`
}

// ChatAPI defines the methods required to interact with the chat backend.
type ChatAPI interface {
	SendMessage(chatID string, sender Sender, content string) (*Message, error)
	UpdateChat(chatID string, updates map[string]interface{}) (*Chat, error)
	GetChat(chatID string) (*Chat, error)
	GetMessages(chatID string) ([]Message, error)
	StartChat(chatID string) (*Chat, error)
	// Add other necessary methods
}

// OllamaAPIClient defines the methods to interact with Ollama API.
type OllamaAPIClient interface {
	Chat(request OllamaChatRequest) (OllamaChatResponse, error)
	// Add other necessary methods
}

// OllamaChatRequest represents the request payload for Ollama's chat endpoint.
type OllamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []OllamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream,omitempty"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// OllamaMessage represents a single message in the Ollama chat request.
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatResponse represents the response from Ollama's chat endpoint.
type OllamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	// Add other relevant fields
}

// HTTPChatAPI is an implementation of ChatAPI using HTTP.
type HTTPChatAPI struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPChatAPI creates a new instance of HTTPChatAPI.
func NewHTTPChatAPI(baseURL string) *HTTPChatAPI {
	return &HTTPChatAPI{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{},
	}
}

// SendMessage sends a message to the chat backend.
func (api *HTTPChatAPI) SendMessage(chatID string, sender Sender, content string) (*Message, error) {
	payload := map[string]interface{}{
		"chatId":  chatID,
		"role":    sender,
		"content": content,
	}
	log.Println("Payload:", payload)
	log.Println("content:", content)
	body, err := json.Marshal(payload)
	if err != nil {
		log.Println("Error marshalling payload:", err)
		return nil, err
	}

	resp, err := api.HTTPClient.Post(fmt.Sprintf("%s/messages", api.BaseURL), "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Println("Error sending message:", err)
		return nil, err
	}
	defer resp.Body.Close()

	var msg Message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		log.Println("Error decoding response:", err)
		return nil, err
	}
	log.Println("Message sent:", msg)
	return &msg, nil
}

// UpdateChat updates chat settings or title.
func (api *HTTPChatAPI) UpdateChat(chatID string, updates map[string]interface{}) (*Chat, error) {
	body, err := json.Marshal(updates)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/chats/%s", api.BaseURL, chatID), bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chat Chat
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		return nil, err
	}

	return &chat, nil
}

// GetChat retrieves a chat by its ID.
func (api *HTTPChatAPI) GetChat(chatID string) (*Chat, error) {
	resp, err := api.HTTPClient.Get(fmt.Sprintf("%s/chats/%s", api.BaseURL, chatID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chat Chat
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		return nil, err
	}

	return &chat, nil
}

// StartChat starts a new chat session.
func (api *HTTPChatAPI) StartChat(chatID string) (*Chat, error) {
	resp, err := api.HTTPClient.Post(fmt.Sprintf("%s/chats", api.BaseURL), "application/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var chat Chat
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil {
		return nil, err
	}

	return &chat, nil
}

// GetMessages retrieves messages for a specific chat.
func (api *HTTPChatAPI) GetMessages(chatID string) ([]Message, error) {
	resp, err := api.HTTPClient.Get(fmt.Sprintf("%s/chats/%s/messages", api.BaseURL, chatID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var messages []Message
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// HTTPollamaAPIClient is an implementation of OllamaAPIClient using HTTP.
type HTTPollamaAPIClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewHTTPollamaAPIClient creates a new instance of HTTPollamaAPIClient.
func NewHTTPollamaAPIClient(baseURL string) *HTTPollamaAPIClient {
	return &HTTPollamaAPIClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{},
	}
}

// Chat sends a chat request to Ollama API.
func (api *HTTPollamaAPIClient) Chat(request OllamaChatRequest) (OllamaChatResponse, error) {
	var response OllamaChatResponse

	body, err := json.Marshal(request)
	if err != nil {
		return response, err
	}

	resp, err := api.HTTPClient.Post(fmt.Sprintf("%s/ollama/chat", api.BaseURL), "application/json", bytes.NewBuffer(body))
	if err != nil {
		return response, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return response, errors.New("failed to get valid response from Ollama API")
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return response, err
	}

	return response, nil
}

// ChatStore manages the chat state.
type ChatStore struct {
	mu          sync.Mutex
	User        *User
	Chat        Chat
	CurrentChat string
	Messages    []Message
	Settings    Settings
	Error       string
	ChatAPI     ChatAPI
	OllamaAPI   OllamaAPIClient
}

// NewChatStore creates a new instance of ChatStore.
func NewChatStore(chatAPI ChatAPI, ollamaAPI OllamaAPIClient) *ChatStore {
	return &ChatStore{
		ChatAPI:   chatAPI,
		OllamaAPI: ollamaAPI,
		Settings:  Settings{}, // Initialize with default settings if necessary
	}
}

// LoadChatStore loads a ChatStore from a given chat ID.
func LoadChatStore(chatID string, chatAPI ChatAPI, ollamaAPI OllamaAPIClient) (*ChatStore, error) {
	chat, err := chatAPI.GetChat(chatID)
	if err != nil {
		return nil, err
	}

	chatStore := NewChatStore(chatAPI, ollamaAPI)
	chatStore.CurrentChat = chatID
	chatStore.Chat = *chat
	chatStore.Messages = chat.Messages
	chatStore.Settings = chat.Settings

	return chatStore, nil
}

// SendMessage sends a message and handles the response from Ollama API.
func (cs *ChatStore) SendMessage(content string) (*OllamaChatResponse, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	log.Println("SendMessage called with content:", content)

	if strings.TrimSpace(cs.CurrentChat) == "" || strings.TrimSpace(content) == "" {
		err := errors.New("current chat ID or content is empty")
		log.Println("Error:", err)
		return nil, err
	}

	// Send user message
	log.Println("Sending user message to ChatAPI")
	userMsg, err := cs.ChatAPI.SendMessage(cs.CurrentChat, SenderUser, content)
	if err != nil {
		cs.Error = err.Error()
		log.Println("Send Message Error:", err)
		return nil, err
	}
	cs.Messages = append(cs.Messages, *userMsg)
	log.Println("User message sent successfully:", userMsg)

	llmSettings := cs.Settings.LLMSettings
	systemPrompt := llmSettings.SystemPrompt
	if systemPrompt == nil {
		systemPrompt = new(string)
		*systemPrompt = ""
	}

	var ollamaMessages []OllamaMessage
	ollamaMessages = append(ollamaMessages, OllamaMessage{
		Role:    "system",
		Content: *systemPrompt,
	})

	for _, msg := range cs.Messages {
		var role string
		switch msg.Role {
		case SenderUser:
			role = "user"
		case SenderAssistant:
			role = "assistant"
		case SenderSystem:
			role = "system"
		default:
			role = "system"
		}
		ollamaMessages = append(ollamaMessages, OllamaMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	fullMessages := ollamaMessages
	log.Println("Prepared full messages for Ollama API:", fullMessages)

	// Filter out nil values from llmSettings
	llmNotNullSettings := make(map[string]interface{})
	if llmSettings.Model != nil {
		llmNotNullSettings["model"] = *llmSettings.Model
	}
	// Add other settings as needed, ensuring to check for non-nil

	// Prepare Ollama chat request
	log.Println("llmNotNullSettings:", llmNotNullSettings)
	ollamaRequest := OllamaChatRequest{
		Model:    *llmSettings.Model,
		Messages: fullMessages,
		Stream:   false,
		Options:  llmNotNullSettings,
	}

	// Send request to Ollama API
	log.Println("Sending request to Ollama API with request:", ollamaRequest)
	response, err := cs.OllamaAPI.Chat(ollamaRequest)
	if err != nil {
		cs.Error = err.Error()
		log.Println("Ollama Chat Error:", err)
		return nil, err
	}
	log.Println("Received response from Ollama API:", response)

	assistantContent := strings.TrimSpace(response.Message.Content)
	if assistantContent == "" {
		err := errors.New("received empty response from Ollama API")
		cs.Error = err.Error()
		log.Println("Empty Response Error:", err)
		return nil, err
	}

	// Send assistant message
	log.Println("Sending assistant message to ChatAPI")
	assistantMsg, err := cs.ChatAPI.SendMessage(cs.CurrentChat, SenderAssistant, assistantContent)
	if err != nil {
		cs.Error = err.Error()
		log.Println("Send Assistant Message Error:", err)
		return nil, err
	}
	cs.Messages = append(cs.Messages, *assistantMsg)
	log.Println("Assistant message sent successfully:", assistantMsg)

	return &response, nil
}
