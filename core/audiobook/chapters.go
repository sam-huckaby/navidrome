// Package audiobook reads the ID3 chapter table of contents without decoding audio.
package audiobook

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf16"
)

type Chapter struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
}

type Book struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	StartMs    int64    `json:"startMs"`
	EndMs      int64    `json:"endMs"`
	ChapterIDs []string `json:"chapterIds"`
}

type Contents struct {
	Chapters []Chapter `json:"chapters,omitempty"`
	Books    []Book    `json:"books,omitempty"`
}

// Read loads only the ID3 header. A missing chapter tag is an empty Contents.
func Read(path string) (Contents, error) {
	f, err := os.Open(path)
	if err != nil {
		return Contents{}, err
	}
	defer f.Close()
	h := make([]byte, 10)
	if _, err = io.ReadFull(f, h); err != nil {
		return Contents{}, err
	}
	if string(h[:3]) != "ID3" || (h[3] != 3 && h[3] != 4) {
		return Contents{}, nil
	}
	size := syncSize(h[6:10])
	if size > 16<<20 {
		return Contents{}, nil
	}
	tag := make([]byte, size)
	if _, err = io.ReadFull(f, tag); err != nil {
		return Contents{}, err
	}
	if h[5]&0x40 != 0 {
		if len(tag) < 4 {
			return Contents{}, nil
		}
		ext := int(binary.BigEndian.Uint32(tag[:4]))
		if h[3] == 4 {
			ext = syncSize(tag[:4])
		}
		if ext < 0 || ext > len(tag) {
			return Contents{}, nil
		}
		tag = tag[ext:]
	}
	contents := Contents{}
	type toc struct {
		title    string
		children []string
		root     bool
	}
	tocs := map[string]toc{}
	chapters := map[string]Chapter{}
	for len(tag) >= 10 {
		kind := string(tag[:4])
		if !frameName(kind) {
			break
		}
		n := int(binary.BigEndian.Uint32(tag[4:8]))
		if h[3] == 4 {
			// Some ID3v2.4 producers (including ffmpeg) emit non-synchsafe frame sizes.
			ss := syncSize(tag[4:8])
			if !frameBoundary(tag, n) && frameBoundary(tag, ss) {
				n = ss
			}
		}
		if n <= 0 || n > len(tag)-10 {
			break
		}
		body := tag[10 : 10+n]
		switch kind {
		case "CHAP":
			if id, rest, ok := cstring(body); ok && len(rest) >= 16 {
				start, end := binary.BigEndian.Uint32(rest[:4]), binary.BigEndian.Uint32(rest[4:8])
				if start < end {
					chapters[id] = Chapter{ID: id, Title: embeddedTitle(rest[16:], h[3]), StartMs: int64(start), EndMs: int64(end)}
				}
			}
		case "CTOC":
			if id, rest, ok := cstring(body); ok && len(rest) >= 2 {
				entry := toc{root: rest[0]&2 != 0}
				count := int(rest[1])
				rest = rest[2:]
				for range count {
					child, next, valid := cstring(rest)
					if !valid {
						break
					}
					entry.children = append(entry.children, child)
					rest = next
				}
				entry.title = embeddedTitle(rest, h[3])
				tocs[id] = entry
			}
		}
		tag = tag[10+n:]
	}
	for _, ch := range chapters {
		contents.Chapters = append(contents.Chapters, ch)
	}
	slices.SortFunc(contents.Chapters, func(a, b Chapter) int {
		if a.StartMs < b.StartMs {
			return -1
		}
		if a.StartMs > b.StartMs {
			return 1
		}
		return 0
	})
	// Only named nested TOCs are books. A flat root is not a book grouping.
	for _, root := range tocs {
		if !root.root {
			continue
		}
		for _, id := range root.children {
			group, ok := tocs[id]
			if !ok || group.title == "" || len(group.children) == 0 {
				continue
			}
			book := Book{ID: id, Title: group.title, ChapterIDs: []string{}}
			for _, child := range group.children {
				if ch, ok := chapters[child]; ok {
					if len(book.ChapterIDs) == 0 {
						book.StartMs = ch.StartMs
					}
					book.EndMs = ch.EndMs
					book.ChapterIDs = append(book.ChapterIDs, child)
				}
			}
			if len(book.ChapterIDs) > 0 {
				contents.Books = append(contents.Books, book)
			}
		}
		break
	}
	slices.SortFunc(contents.Books, func(a, b Book) int {
		if a.StartMs < b.StartMs {
			return -1
		}
		if a.StartMs > b.StartMs {
			return 1
		}
		return 0
	})
	return contents, nil
}

func Decode(data string) Contents {
	var result Contents
	_ = json.Unmarshal([]byte(data), &result)
	return result
}

func syncSize(b []byte) int { return int(b[0])<<21 | int(b[1])<<14 | int(b[2])<<7 | int(b[3]) }
func frameName(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
func frameBoundary(tag []byte, n int) bool {
	return n >= 0 && n+10 <= len(tag) && (n+10 == len(tag) || len(tag[n+10:]) < 4 || frameName(string(tag[n+10:n+14])) || tag[n+10] == 0)
}
func cstring(b []byte) (string, []byte, bool) {
	if i := slices.Index(b, byte(0)); i >= 0 {
		return string(b[:i]), b[i+1:], true
	}
	return "", nil, false
}
func embeddedTitle(b []byte, version byte) string {
	for len(b) >= 10 {
		kind := string(b[:4])
		if !frameName(kind) {
			break
		}
		n := int(binary.BigEndian.Uint32(b[4:8]))
		if version == 4 && !frameBoundary(b, n) && frameBoundary(b, syncSize(b[4:8])) {
			n = syncSize(b[4:8])
		}
		if n <= 0 || n > len(b)-10 {
			break
		}
		if kind == "TIT2" && n > 1 {
			raw := b[11 : 10+n]
			if b[10] == 0 || b[10] == 3 {
				return strings.TrimRight(string(raw), "\x00")
			}
			if b[10] == 1 || b[10] == 2 {
				little := b[10] == 1 && len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xfe
				if b[10] == 1 && len(raw) >= 2 {
					raw = raw[2:]
				}
				var chars []uint16
				for len(raw) >= 2 {
					if little {
						chars = append(chars, binary.LittleEndian.Uint16(raw))
					} else {
						chars = append(chars, binary.BigEndian.Uint16(raw))
					}
					raw = raw[2:]
				}
				return strings.TrimRight(string(utf16.Decode(chars)), "\x00")
			}
		}
		b = b[10+n:]
	}
	return ""
}
