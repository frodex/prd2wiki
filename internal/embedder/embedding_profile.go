package embedder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// EmbeddingProfile is an immutable descriptor of how embeddings were generated.
// It travels with federation bundles so Stewards can compare and re-embed when
// dimensions, model, or metric differ between peers.
type EmbeddingProfile struct {
	ProfileID     string    `json:"profile_id,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	Provider      string    `json:"provider"`
	ModelID       string    `json:"model_id"`
	ModelVersion  string    `json:"model_version"`
	Dimensions    int       `json:"dimensions"`
	Metric        string    `json:"metric"` // "cosine", "dot", "l2"
	Normalized    bool      `json:"normalized"`
	TextPrep      TextPrep  `json:"text_prep"`
	QueryPrefix   string    `json:"query_prefix"`
	PassagePrefix string    `json:"passage_prefix"`
}

type TextPrep struct {
	TrimWhitespace bool   `json:"trim_whitespace"`
	CaseFold       bool   `json:"case_fold"`
	UnicodeNorm    string `json:"unicode_norm"` // "NFC" | "NFKC" | "none"
	MaxTokens      int    `json:"max_tokens"`
	Chunking       string `json:"chunking"`
}

type ProfileMatch string

const (
	ProfileExactMatch      ProfileMatch = "exact_match"
	ProfileVersionMismatch ProfileMatch = "version_mismatch"
	ProfileIncompatible    ProfileMatch = "incompatible"
	ProfileMissing         ProfileMatch = "missing"
)

// Compare returns the federation import action class defined in steward spec.
func (p *EmbeddingProfile) Compare(other *EmbeddingProfile) ProfileMatch {
	if p == nil || other == nil {
		return ProfileMissing
	}
	if p.Provider != other.Provider || p.ModelID != other.ModelID {
		return ProfileIncompatible
	}
	if p.Dimensions != other.Dimensions || p.Metric != other.Metric || p.Normalized != other.Normalized {
		return ProfileIncompatible
	}
	if p.TextPrep != other.TextPrep {
		return ProfileIncompatible
	}
	if p.ModelVersion != other.ModelVersion {
		return ProfileVersionMismatch
	}
	return ProfileExactMatch
}

type embeddingProfileWire struct {
	Provider      string   `json:"provider"`
	ModelID       string   `json:"model_id"`
	ModelVersion  string   `json:"model_version"`
	Dimensions    int      `json:"dimensions"`
	Metric        string   `json:"metric"`
	Normalized    bool     `json:"normalized"`
	TextPrep      TextPrep `json:"text_prep"`
	QueryPrefix   string   `json:"query_prefix"`
	PassagePrefix string   `json:"passage_prefix"`
}

type embeddingProfileLegacyWire struct {
	ModelName     string `json:"model_name"`
	ModelVersion  string `json:"model_version"`
	Dimensions    int    `json:"dimensions"`
	MaxTokens     int    `json:"max_tokens"`
	Metric        string `json:"metric"`
	Normalization string `json:"normalization"`
	QueryPrefix   string `json:"query_prefix"`
	PassagePrefix string `json:"passage_prefix"`
	Preprocessing string `json:"preprocessing"`
}

func canonicalProfileID(provider, modelID string, dims int) string {
	return fmt.Sprintf("%s_%s_%d", provider, modelID, dims)
}

func defaultTextPrep() TextPrep {
	return TextPrep{
		TrimWhitespace: true,
		CaseFold:       false,
		UnicodeNorm:    "NFC",
		MaxTokens:      8192,
		Chunking:       "none",
	}
}

func normalizeProfile(p *EmbeddingProfile) *EmbeddingProfile {
	if p == nil {
		return nil
	}
	if p.Provider == "" {
		p.Provider = "unknown"
	}
	if p.ModelID == "" {
		p.ModelID = "unknown"
	}
	if p.Metric == "" {
		p.Metric = "cosine"
	}
	if p.TextPrep.UnicodeNorm == "" {
		p.TextPrep.UnicodeNorm = "NFC"
	}
	if p.TextPrep.MaxTokens <= 0 {
		p.TextPrep.MaxTokens = 8192
	}
	if p.TextPrep.Chunking == "" {
		p.TextPrep.Chunking = "none"
	}
	if p.ProfileID == "" {
		p.ProfileID = canonicalProfileID(p.Provider, p.ModelID, p.Dimensions)
	}
	return p
}

func legacyNormalizationToBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "l2", "unit", "normalized", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func textPrepJSON(tp TextPrep) string {
	b, err := json.Marshal(tp)
	if err != nil {
		return `{}`
	}
	return string(b)
}

func parseTextPrep(raw string) TextPrep {
	tp := defaultTextPrep()
	if strings.TrimSpace(raw) == "" {
		return tp
	}
	var parsed TextPrep
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if parsed.UnicodeNorm == "" {
			parsed.UnicodeNorm = tp.UnicodeNorm
		}
		if parsed.MaxTokens <= 0 {
			parsed.MaxTokens = tp.MaxTokens
		}
		if parsed.Chunking == "" {
			parsed.Chunking = tp.Chunking
		}
		return parsed
	}
	return tp
}

func preprocessingToTextPrep(preprocessing string, maxTokens int) TextPrep {
	tp := defaultTextPrep()
	if maxTokens > 0 {
		tp.MaxTokens = maxTokens
	}
	switch strings.ToLower(strings.TrimSpace(preprocessing)) {
	case "", "nfc_trim":
		return tp
	default:
		return tp
	}
}

// EmbeddingProfileStore manages immutable embedding profiles in SQLite.
type EmbeddingProfileStore struct {
	db *sql.DB
}

func NewEmbeddingProfileStore(db *sql.DB) (*EmbeddingProfileStore, error) {
	if err := migrateProfiles(db); err != nil {
		return nil, fmt.Errorf("embedding_profiles migration: %w", err)
	}
	return &EmbeddingProfileStore{db: db}, nil
}

// migrateProfiles creates the embedding_profiles table and adds columns if needed.
func migrateProfiles(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS embedding_profiles (
		profile_id TEXT PRIMARY KEY,
		provider TEXT NOT NULL DEFAULT 'unknown',
		model_id TEXT NOT NULL DEFAULT '',
		model_name TEXT NOT NULL,
		model_version TEXT NOT NULL DEFAULT '',
		dimensions INTEGER NOT NULL,
		max_tokens INTEGER NOT NULL DEFAULT 0,
		metric TEXT NOT NULL DEFAULT 'cosine',
		normalized INTEGER NOT NULL DEFAULT 1,
		normalization TEXT NOT NULL DEFAULT 'l2',
		text_prep_json TEXT NOT NULL DEFAULT '{}',
		query_prefix TEXT NOT NULL DEFAULT '',
		passage_prefix TEXT NOT NULL DEFAULT '',
		preprocessing TEXT NOT NULL DEFAULT 'nfc_trim',
		created_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return err
	}

	// Add columns that may not exist yet (idempotent).
	columns := map[string]string{
		"provider":       "TEXT NOT NULL DEFAULT 'unknown'",
		"model_id":       "TEXT NOT NULL DEFAULT ''",
		"normalized":     "INTEGER NOT NULL DEFAULT 1",
		"text_prep_json": "TEXT NOT NULL DEFAULT '{}'",
	}
	for col, colDef := range columns {
		// Check if column exists by querying table_info.
		var exists bool
		rows, err := db.Query("PRAGMA table_info(embedding_profiles)")
		if err != nil {
			return err
		}
		for rows.Next() {
			var cid int
			var name, colType string
			var notNull int
			var dfltValue sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == col {
				exists = true
			}
		}
		rows.Close()
		if !exists {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE embedding_profiles ADD COLUMN %s %s", col, colDef)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Register stores a new profile. Returns an error if the profile_id already exists.
func (s *EmbeddingProfileStore) Register(ctx context.Context, p *EmbeddingProfile) error {
	if p == nil || p.ProfileID == "" {
		return fmt.Errorf("profile_id is required")
	}
	p = normalizeProfile(p)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO embedding_profiles (
			profile_id, provider, model_id, model_name, model_version,
			dimensions, max_tokens, metric, normalized, normalization, text_prep_json,
			query_prefix, passage_prefix, preprocessing, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ProfileID, p.Provider, p.ModelID, p.ModelID, p.ModelVersion,
		p.Dimensions, p.TextPrep.MaxTokens, p.Metric, p.Normalized, "l2", textPrepJSON(p.TextPrep),
		p.QueryPrefix, p.PassagePrefix, "nfc_trim", p.CreatedAt,
	)
	return err
}

// Get retrieves a profile by ID.
func (s *EmbeddingProfileStore) Get(ctx context.Context, id string) (*EmbeddingProfile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			profile_id, provider, model_id, model_name, model_version, dimensions, max_tokens, metric,
			normalized, normalization, text_prep_json, query_prefix, passage_prefix, preprocessing, created_at
		FROM embedding_profiles WHERE profile_id = ?`, id)
	var p EmbeddingProfile
	var provider, modelID, legacyModelName, legacyNormalization, textPrepRaw, preprocessing string
	var maxTokens int
	var normalized int
	err := row.Scan(
		&p.ProfileID, &provider, &modelID, &legacyModelName, &p.ModelVersion, &p.Dimensions, &maxTokens, &p.Metric,
		&normalized, &legacyNormalization, &textPrepRaw, &p.QueryPrefix, &p.PassagePrefix, &preprocessing, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if provider == "" || provider == "unknown" {
		provider = "llama_cpp"
	}
	if modelID == "" {
		modelID = legacyModelName
	}
	p.Provider = provider
	p.ModelID = modelID
	p.Normalized = normalized != 0 || legacyNormalizationToBool(legacyNormalization)
	p.TextPrep = parseTextPrep(textPrepRaw)
	if strings.TrimSpace(textPrepRaw) == "" {
		p.TextPrep = preprocessingToTextPrep(preprocessing, maxTokens)
	}
	return normalizeProfile(&p), nil
}

// List returns all profiles ordered by creation time descending.
func (s *EmbeddingProfileStore) List(ctx context.Context) ([]*EmbeddingProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			profile_id, provider, model_id, model_name, model_version, dimensions, max_tokens, metric,
			normalized, normalization, text_prep_json, query_prefix, passage_prefix, preprocessing, created_at
		FROM embedding_profiles ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []*EmbeddingProfile
	for rows.Next() {
		var p EmbeddingProfile
		var provider, modelID, legacyModelName, legacyNormalization, textPrepRaw, preprocessing string
		var maxTokens int
		var normalized int
		if err := rows.Scan(
			&p.ProfileID, &provider, &modelID, &legacyModelName, &p.ModelVersion, &p.Dimensions, &maxTokens, &p.Metric,
			&normalized, &legacyNormalization, &textPrepRaw, &p.QueryPrefix, &p.PassagePrefix, &preprocessing, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		if provider == "" || provider == "unknown" {
			provider = "llama_cpp"
		}
		if modelID == "" {
			modelID = legacyModelName
		}
		p.Provider = provider
		p.ModelID = modelID
		p.Normalized = normalized != 0 || legacyNormalizationToBool(legacyNormalization)
		p.TextPrep = parseTextPrep(textPrepRaw)
		if strings.TrimSpace(textPrepRaw) == "" {
			p.TextPrep = preprocessingToTextPrep(preprocessing, maxTokens)
		}
		profiles = append(profiles, normalizeProfile(&p))
	}
	return profiles, rows.Err()
}

// ProfileFromConfig creates an EmbeddingProfile from an EmbedderConfig.
func ProfileFromConfig(cfg EmbedderConfig) *EmbeddingProfile {
	p := &EmbeddingProfile{
		Provider:      cfg.Type,
		ModelID:       cfg.Model,
		ModelVersion:  "",
		Dimensions:    cfg.Dimensions,
		Metric:        "cosine",
		Normalized:    true,
		TextPrep:      defaultTextPrep(),
		QueryPrefix:   cfg.QueryPrefix,
		PassagePrefix: cfg.PassagePrefix,
		CreatedAt:     time.Now().UTC(),
	}
	return normalizeProfile(p)
}

// MarshalProfile serializes a profile to JSON for inclusion in federation bundles.
func MarshalProfile(p *EmbeddingProfile) (string, error) {
	if p == nil {
		return "", fmt.Errorf("nil embedding profile")
	}
	p = normalizeProfile(p)
	wire := embeddingProfileWire{
		Provider:      p.Provider,
		ModelID:       p.ModelID,
		ModelVersion:  p.ModelVersion,
		Dimensions:    p.Dimensions,
		Metric:        p.Metric,
		Normalized:    p.Normalized,
		TextPrep:      p.TextPrep,
		QueryPrefix:   p.QueryPrefix,
		PassagePrefix: p.PassagePrefix,
	}
	b, err := json.Marshal(wire)
	return string(b), err
}

// UnmarshalProfile deserializes a profile from a federation bundle.
func UnmarshalProfile(data string) (*EmbeddingProfile, error) {
	var wire embeddingProfileWire
	if err := json.Unmarshal([]byte(data), &wire); err == nil && (wire.Provider != "" || wire.ModelID != "") {
		p := &EmbeddingProfile{
			Provider:      wire.Provider,
			ModelID:       wire.ModelID,
			ModelVersion:  wire.ModelVersion,
			Dimensions:    wire.Dimensions,
			Metric:        wire.Metric,
			Normalized:    wire.Normalized,
			TextPrep:      wire.TextPrep,
			QueryPrefix:   wire.QueryPrefix,
			PassagePrefix: wire.PassagePrefix,
		}
		return normalizeProfile(p), nil
	}
	var legacy embeddingProfileLegacyWire
	if err := json.Unmarshal([]byte(data), &legacy); err != nil {
		return nil, err
	}
	p := &EmbeddingProfile{
		Provider:      "llama_cpp",
		ModelID:       legacy.ModelName,
		ModelVersion:  legacy.ModelVersion,
		Dimensions:    legacy.Dimensions,
		Metric:        legacy.Metric,
		Normalized:    legacyNormalizationToBool(legacy.Normalization),
		TextPrep:      preprocessingToTextPrep(legacy.Preprocessing, legacy.MaxTokens),
		QueryPrefix:   legacy.QueryPrefix,
		PassagePrefix: legacy.PassagePrefix,
	}
	return normalizeProfile(p), nil
}
