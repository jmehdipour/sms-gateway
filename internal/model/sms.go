package model

import "strings"

type SMSType string

const (
	SMSTypeNormal  SMSType = "normal"
	SMSTypeExpress SMSType = "express"
)

func (t SMSType) String() string { return string(t) }

// ParseSMSType normalizes input; empty => normal.
// Returns (value, true) if valid; otherwise (normal, false).
func ParseSMSType(s string) (SMSType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "normal":
		return SMSTypeNormal, true
	case "express":
		return SMSTypeExpress, true
	default:
		return SMSTypeNormal, false
	}
}

func (t SMSType) Valid() bool {
	return t == SMSTypeNormal || t == SMSTypeExpress
}

type SMS struct {
	Phone string  `json:"phone"`
	Text  string  `json:"text"`
	Type  SMSType `json:"type,omitempty"` // "normal" | "express"
}
