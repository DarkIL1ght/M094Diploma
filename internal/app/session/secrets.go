package session

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strings"
)

// TokenHash — SHA-256 токена сессии: так токен лежит в хранилище.
type TokenHash [32]byte

// KeyHash — SHA-256 нормализованного ключа восстановления.
type KeyHash [32]byte

const tokenBytes = 32

// newToken — 256 случайных бит в base64url (43 символа).
func newToken(r io.Reader) (string, error) {
	var b [tokenBytes]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// validToken отсекает мусор из cookie до обращения к хранилищу.
func validToken(t string) bool {
	if len(t) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	b, err := base64.RawURLEncoding.DecodeString(t)
	return err == nil && len(b) == tokenBytes
}

// HashToken считает хэш токена. Токен случайный и длинный, поэтому медленная функция не нужна.
func HashToken(t string) TokenHash { return sha256.Sum256([]byte(t)) }

// Алфавит ключа — Crockford base32 без I, L, O, U: символы не путаются при переписывании от руки.
const (
	keyAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	keyChars    = 16 // 16 символов по 5 бит = 80 бит
)

// newRecoveryKey возвращает ключ вида K7QM-4XD9-2PNA-WT6H.
func newRecoveryKey(r io.Reader) (string, error) {
	var raw [keyChars]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, b := range raw {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(keyAlphabet[b&31]) // 256 делится на 32: перекоса нет
	}
	return sb.String(), nil
}

// normalizeKey приводит введённый ключ к каноническому виду: без дефисов и пробелов, в верхнем регистре,
// похожие символы заменены (O→0, I и L→1). Не подходящий по алфавиту и длине ключ — ok=false.
func normalizeKey(s string) (string, bool) {
	var sb strings.Builder
	for _, c := range strings.ToUpper(s) {
		switch c {
		case '-', ' ', '_', '\t', '\n', '\r':
			continue
		case 'O':
			c = '0'
		case 'I', 'L':
			c = '1'
		}
		if c > 127 || !strings.ContainsRune(keyAlphabet, c) {
			return "", false
		}
		sb.WriteRune(c)
	}
	if sb.Len() != keyChars {
		return "", false
	}
	return sb.String(), true
}

// HashKey нормализует ключ и считает хэш. ok=false — ключ неверного формата.
// 80 бит случайности не перебрать, поэтому хватает SHA-256; от подбора защищает ограничение частоты.
func HashKey(key string) (KeyHash, bool) {
	n, ok := normalizeKey(key)
	if !ok {
		return KeyHash{}, false
	}
	return sha256.Sum256([]byte(n)), true
}

func randomHex(r io.Reader, n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// displayName — случайный ник вида «Быстрый Барс» для лиги недели. Берём 32 бита на слово:
// остаток от деления на 12 при таком числе даёт перекос порядка 1e-9.
func displayName(r io.Reader) (string, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	adj := binary.BigEndian.Uint32(b[:4]) % uint32(len(nameAdjectives))
	animal := binary.BigEndian.Uint32(b[4:]) % uint32(len(nameAnimals))
	return nameAdjectives[adj] + " " + nameAnimals[animal], nil
}

// Прилагательные и существительные мужского рода: согласуются в любой паре.
var (
	nameAdjectives = []string{"Быстрый", "Тихий", "Смелый", "Хитрый", "Весёлый", "Упрямый", "Зоркий", "Ловкий", "Спокойный", "Дерзкий", "Мудрый", "Яркий"}
	nameAnimals    = []string{"Барс", "Волк", "Лис", "Ёж", "Тигр", "Орёл", "Бобр", "Сокол", "Медведь", "Кабан", "Бык", "Соболь"}
)
