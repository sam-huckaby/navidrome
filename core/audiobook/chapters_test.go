package audiobook

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"go.senan.xyz/taglib"
)

func TestReadHierarchicalID3(t *testing.T) {
	frame := func(kind string, data []byte) []byte {
		result := make([]byte, 10+len(data))
		copy(result, kind)
		binary.BigEndian.PutUint32(result[4:], uint32(len(data)))
		copy(result[10:], data)
		return result
	}
	chapter := func(id string, start, end uint32) []byte {
		data := append([]byte(id+"\x00"), make([]byte, 16)...)
		binary.BigEndian.PutUint32(data[len(id)+1:], start)
		binary.BigEndian.PutUint32(data[len(id)+5:], end)
		return frame("CHAP", append(data, frame("TIT2", append([]byte{3}, []byte("Chapter "+id)...))...))
	}
	root := frame("CTOC", []byte("toc\x00\x03\x01book\x00"))
	book := frame("CTOC", append([]byte("book\x00\x01\x02ch1\x00ch2\x00"), frame("TIT2", []byte("\x03Example book"))...))
	tag := append(append(append(root, book...), chapter("ch1", 1000, 2000)...), chapter("ch2", 2000, 3000)...)
	h := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")
	h[6] = byte(len(tag) >> 21 & 127)
	h[7] = byte(len(tag) >> 14 & 127)
	h[8] = byte(len(tag) >> 7 & 127)
	h[9] = byte(len(tag) & 127)
	path := filepath.Join(t.TempDir(), "test.mp3")
	if err := os.WriteFile(path, append(h, tag...), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chapters) != 2 || got.Chapters[0].Title != "Chapter ch1" || len(got.Books) != 1 || got.Books[0].Title != "Example book" || len(got.Books[0].ChapterIDs) != 2 {
		t.Fatalf("unexpected contents: %+v", got)
	}
}

func TestReadSample(t *testing.T) {
	path := os.Getenv("AUDIOBOOK_SAMPLE_PATH")
	if path == "" {
		t.Skip("set AUDIOBOOK_SAMPLE_PATH for real MP3 check")
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Chapters) != 113 || len(got.Books) != 7 || got.Books[0].StartMs != 41192 {
		t.Fatalf("unexpected contents: %+v", got)
	}
	expectedStarts := []int64{41192, 14213213, 29890165, 46691799, 63504579, 84544319, 104077967}
	expectedTitles := []string{"The Magician’s Nephew", "The Lion, the Witch and the Wardrobe", "The Horse and His Boy", "Prince Caspian", "The Voyage of the Dawn Treader", "The Silver Chair", "The Last Battle"}
	for i, book := range got.Books {
		if book.StartMs != expectedStarts[i] || book.Title != expectedTitles[i] {
			t.Fatalf("book %d: %+v", i, book)
		}
	}
	tags, err := taglib.ReadTags(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags["RELEASETYPE"]) == 0 {
		t.Fatalf("release type missing: %v", tags)
	}
}

// Run against a disposable copy of the real MP3 to check the importer's tag
// writer preserves unknown CHAP/CTOC frames when it fills missing tags.
func TestUploadTagRewrite(t *testing.T) {
	path := os.Getenv("AUDIOBOOK_RETAG_COPY")
	if path == "" {
		t.Skip("set AUDIOBOOK_RETAG_COPY to a disposable MP3 copy")
	}
	if err := taglib.WriteTags(path, map[string][]string{"COMMENT": {"upload metadata verification"}}, 0); err != nil {
		t.Fatal(err)
	}
	contents, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Chapters) != 113 || len(contents.Books) != 7 {
		t.Fatalf("tag rewrite lost chapters or books: %d chapters, %d books", len(contents.Chapters), len(contents.Books))
	}
}
