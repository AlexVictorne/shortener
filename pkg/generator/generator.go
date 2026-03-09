package generator

import (
	"crypto/rand"
	"encoding/base64"
)

type Generator struct {
	length int
}

func NewGenerator(length int) IDGenerator {
	return &Generator{length: length}
}

func (g *Generator) GenerateID() (string, error) {
	bytes := make([]byte, g.length)
	if _, error := rand.Read(bytes); error != nil {
		return "", error
	}

	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	if len(encoded) > g.length {
		return encoded[:g.length], nil
	}

	return encoded, nil
}

func (g *Generator) Validate(id string) bool {
	if len(id) != g.length {
		return false
	}

	for _, ch := range id {
		if !isURLSafeChar(ch) {
			return false
		}
	}

	return true
}

func isURLSafeChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' || ch == '-'
}
