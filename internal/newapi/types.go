package newapi

import "encoding/json"

type Response[T any] struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    T               `json:"data"`
}

type ChannelList struct {
	Items     []Channel `json:"items"`
	Total     int64     `json:"total"`
	Page      int       `json:"page"`
	PageSize  int       `json:"page_size"`
	TypeCount map[int64]int64 `json:"type_counts"`
}

type Channel struct {
	ID           int             `json:"id"`
	Type         int             `json:"type"`
	Status       int             `json:"status"`
	Name         string          `json:"name"`
	Weight       *uint           `json:"weight"`
	Priority     *int64          `json:"priority"`
	BaseURL      *string         `json:"base_url"`
	Models       string          `json:"models"`
	Group        string          `json:"group"`
	ModelMapping *string         `json:"model_mapping"`
	Tag          *string         `json:"tag"`
	Other        json.RawMessage `json:"other,omitempty"`
}

type LogList struct {
	Items    []Log `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

type Log struct {
	ID                int             `json:"id"`
	CreatedAt         int64           `json:"created_at"`
	Type              int             `json:"type"`
	Content           string          `json:"content"`
	ModelName         string          `json:"model_name"`
	Quota             int             `json:"quota"`
	PromptTokens      int             `json:"prompt_tokens"`
	CompletionTokens  int             `json:"completion_tokens"`
	UseTime           int             `json:"use_time"`
	IsStream          bool            `json:"is_stream"`
	Channel           int             `json:"channel"`
	Group             string          `json:"group"`
	RequestID         string          `json:"request_id"`
	UpstreamRequestID string          `json:"upstream_request_id"`
	Other             json.RawMessage `json:"other"`
}

type LogStat struct {
	Quota int `json:"quota"`
	RPM   int `json:"rpm"`
	TPM   int `json:"tpm"`
}

