package models

import (
	"time"

	"github.com/fish0607/go-wecom/commons/gorms"
)

type Token struct {
	gorms.Model
	Type      string
	CorpID    string
	ExpiredAt *time.Time
}
