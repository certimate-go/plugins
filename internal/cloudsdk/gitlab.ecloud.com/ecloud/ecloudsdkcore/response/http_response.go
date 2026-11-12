package response

type HttpResponse struct {
	StatusCode int                 `json:"statusCode,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Data       interface{}         `json:"data,omitempty"`
}
