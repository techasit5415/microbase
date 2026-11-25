package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// GenerateFromPattern produces a cryptographically random string matching a PocketBase autogenerate pattern.
// Examples:
//   - "[a-z0-9]{15}" -> "k7m2p9x4a1q8w3z"
//   - "usr_[A-Z0-9]{8}" -> "usr_9F4K2M1Q"
//   - "uuid" -> RFC4122 UUIDv4
func GenerateFromPattern(pattern string) string {
	p := strings.TrimSpace(pattern)
	if p == "" {
		p = "[a-z0-9]{15}"
	}
	if strings.EqualFold(p, "uuid") {
		return generateUUIDv4()
	}

	var out strings.Builder
	for i := 0; i < len(p); {
		if p[i] == '[' {
			end := strings.IndexByte(p[i:], ']')
			if end != -1 {
				charset := expandCharClass(p[i+1 : i+end])
				i += end + 1
				repeat := 1
				if i < len(p) && p[i] == '{' {
					closeBrace := strings.IndexByte(p[i:], '}')
					if closeBrace != -1 {
						spec := p[i+1 : i+closeBrace]
						parts := strings.Split(spec, ",")
						minN, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
						maxN := minN
						if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
							if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && v >= minN {
								maxN = v
							}
						}
						if minN < 1 {
							minN = 1
						}
						if maxN < minN {
							maxN = minN
						}
						if maxN > 128 {
							maxN = 128
						}
						repeat = minN
						if maxN > minN {
							repeat = minN + cryptoRandInt(maxN-minN+1)
						}
						i += closeBrace + 1
					}
				}
				for r := 0; r < repeat; r++ {
					out.WriteByte(charset[cryptoRandInt(len(charset))])
				}
				continue
			}
		}
		if p[i] == '\\' && i+1 < len(p) {
			switch p[i+1] {
			case 'd':
				out.WriteByte("0123456789"[cryptoRandInt(10)])
			case 'w':
				const w = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
				out.WriteByte(w[cryptoRandInt(len(w))])
			default:
				out.WriteByte(p[i+1])
			}
			i += 2
			continue
		}
		out.WriteByte(p[i])
		i++
	}
	return out.String()
}

func expandCharClass(spec string) string {
	var b strings.Builder
	for i := 0; i < len(spec); i++ {
		if i+2 < len(spec) && spec[i+1] == '-' {
			start, end := spec[i], spec[i+2]
			for c := start; c <= end; c++ {
				b.WriteByte(c)
			}
			i += 2
		} else {
			b.WriteByte(spec[i])
		}
	}
	if b.Len() == 0 {
		return "abcdefghijklmnopqrstuvwxyz0123456789"
	}
	return b.String()
}

func cryptoRandInt(max int) int {
	if max <= 1 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func generateUUIDv4() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6] = (u[6] & 0x0f) | 0x40
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
