package vectordb

type PageEmbedding struct {
	PageID  string    `json:"page_id"`
	Section string    `json:"section"`
	Project string    `json:"project"`
	Type    string    `json:"type"`
	Tags    string    `json:"tags"`
	Vector  []float32 `json:"vector"`
	Text    string    `json:"text"`
}

type TextChunk struct {
	Section string
	Text    string
}

type SearchResult struct {
	PageID     string  `json:"page_id"`
	Section    string  `json:"section"`
	Similarity float64 `json:"similarity"`
	Text       string  `json:"text"`
}
