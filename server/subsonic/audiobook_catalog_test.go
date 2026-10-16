package subsonic

import (
	"github.com/navidrome/navidrome/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Audiobook catalog classification", func() {
	It("uses a real album artist or the predominant chapter artist, then Unknown Author", func() {
		books := model.Albums{
			{ID: "tagged", AlbumArtist: "S. D. Smith", AlbumArtistID: "smith"},
			{ID: "inconsistent", AlbumArtist: "Various Artists", AlbumArtistID: "various"},
			{ID: "untagged", AlbumArtist: "Unknown Artist"},
		}
		tracks := model.MediaFiles{
			{AlbumID: "inconsistent", ArtistID: "misspelled", Artist: "Cerventas"},
			{AlbumID: "inconsistent", ArtistID: "cervantes", Artist: "Cervantes"},
			{AlbumID: "inconsistent", ArtistID: "cervantes", Artist: "Cervantes"},
			{AlbumID: "inconsistent", ArtistID: "cervantes", Artist: "Cervantes", Missing: true},
			{AlbumID: "tagged", ArtistID: "narrator", Artist: "Narrator"},
		}
		authors := resolveBookAuthors(books, tracks)
		Expect(authors["tagged"].id).To(Equal("smith"))
		Expect(authors["inconsistent"].id).To(Equal("cervantes"))
		Expect(authors["untagged"].id).To(Equal(unknownAudiobookAuthor))
	})

	It("treats an audiobook-only genre as Uncategorized without hiding subject genres", func() {
		Expect(bookGenres(model.Album{Genres: model.Genres{{Name: "Audiobook"}}})).To(BeEmpty())
		Expect(bookGenres(model.Album{Genres: model.Genres{{Name: "Audiobook"}, {Name: "Fantasy"}, {Name: "fantasy"}}})).To(Equal([]string{"Fantasy"}))
	})
})
