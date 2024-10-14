package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type User struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // Exclude from JSON for security
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Chats     []Chat    `json:"chats,omitempty"`
}
type Chat struct {
	ID          string                 `json:"id"`
	UserID      uint                   `json:"userId"`
	StartTime   time.Time              `json:"startTime"`
	EndTime     *time.Time             `json:"endTime,omitempty"`
	Messages    []Message              `json:"messages,omitempty"`
	Settings    map[string]interface{} `json:"settings,omitempty"`
	STTSettings STTSettings            `json:"sttSettings,omitempty"`
	LLMSettings LLMSettings            `json:"llmSettings,omitempty"`
	TTSSettings map[string]interface{} `json:"ttsSettings,omitempty"`
}

type STTSettings struct {
	Language                      string  `json:"language"`
	BeamSize                      int     `json:"beam_size,omitempty"`
	BestOf                        int     `json:"best_of,omitempty"`
	Patience                      float64 `json:"patience,omitempty"`
	NoSpeechThreshold             float64 `json:"no_speech_threshold,omitempty"`
	Temperature                   float64 `json:"temperature,omitempty"`
	HallucinationSilenceThreshold float64 `json:"hallucination_silence_threshold,omitempty"`
}

type LLMSettings struct {
	Model         string  `json:"model"`
	SystemPrompt  string  `json:"system_prompt"`
	Mirostat      int     `json:"mirostat"`
	MirostatEta   int     `json:"mirostat_eta"`
	MirostatTau   int     `json:"mirostat_tau"`
	NumCtx        int     `json:"num_ctx"`
	RepeatLastN   int     `json:"repeat_last_n"`
	RepeatPenalty float64 `json:"repeat_penalty"`
	Temperature   float64 `json:"temperature"`
	TfsZ          float64 `json:"tfs_z"`
	NumPredict    int     `json:"num_predict"`
	TopK          int     `json:"top_k"`
	TopP          float64 `json:"top_p"`
	MinP          float64 `json:"min_p"`
}

type Message struct {
	ID      uint      `json:"id"`
	ChatID  string    `json:"chatId"`
	Role    string    `json:"role"`
	Content string    `json:"content"`
	SentAt  time.Time `json:"sentAt"`
}

var (
	UserSender string = "user"
	LLMSender  string = "assistant"
)

type ChatAPI struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewChatAPI(baseURL string) *ChatAPI {
	return &ChatAPI{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{},
	}
}

// Users
func (api *ChatAPI) CreateUser(username, email string) (map[string]interface{}, error) {
	data := map[string]string{"username": username, "email": email}
	return api.post("/users", data)
}

func (api *ChatAPI) ListUsers() (map[string]interface{}, error) {
	return api.get("/users")
}

// Chats
func (api *ChatAPI) StartChat(userID string) (map[string]interface{}, error) {
	data := map[string]string{"userId": userID}
	return api.post("/chats", data)
}

func (api *ChatAPI) GetChat(chatID string) (*Chat, error) {
	resp, err := api.HTTPClient.Get(api.BaseURL + "/chats/" + chatID)
	if err != nil {
		return nil, fmt.Errorf("error fetching chat: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get chat: received status code %d", resp.StatusCode)
	}

	var chat Chat
	if err := parseJSONResponse(resp.Body, &chat); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %v", err)
	}

	return &chat, nil
}

func (api *ChatAPI) ListChats() (map[string]interface{}, error) {
	return api.get("/chats")
}

func (api *ChatAPI) DeleteChat(chatID string) (map[string]interface{}, error) {
	return api.delete(fmt.Sprintf("/chats/%s", chatID))
}

func (api *ChatAPI) UpdateChat(chatID string, data map[string]interface{}) (map[string]interface{}, error) {
	return api.put(fmt.Sprintf("/chats/%s", chatID), data)
}

// Messages
func (api *ChatAPI) SendMessage(chatID, sender, content string) (map[string]interface{}, error) {
	data := map[string]string{"chatId": chatID, "role": sender, "content": content}
	return api.post("/messages", data)
}

func (api *ChatAPI) GetMessages(chatID string) (map[string]interface{}, error) {
	return api.get(fmt.Sprintf("/messages/%s", chatID))
}

// Helper methods
func (api *ChatAPI) get(path string) (map[string]interface{}, error) {
	resp, err := api.HTTPClient.Get(api.BaseURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseResponse(resp.Body)
}

func (api *ChatAPI) post(path string, data interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	resp, err := api.HTTPClient.Post(api.BaseURL+path, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseResponse(resp.Body)
}

func (api *ChatAPI) put(path string, data interface{}) (map[string]interface{}, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPut, api.BaseURL+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseResponse(resp.Body)
}

func (api *ChatAPI) delete(path string) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodDelete, api.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return parseResponse(resp.Body)
}

func parseResponse(body io.Reader) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := parseJSONResponse(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func parseJSONResponse(body io.Reader, v interface{}) error {
	decoder := json.NewDecoder(body)
	return decoder.Decode(v)
}
