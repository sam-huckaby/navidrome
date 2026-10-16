package subsonic

import (
	"context"
	"net/http/httptest"

	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Audiobook chapter ordering", func() {
	It("orders by disc and track before title, falling back to title when tags are absent", func() {
		ds := &tests.MockDataStore{}
		ds.Album().(*tests.MockAlbumRepo).SetData(model.Albums{{ID: "book", IsAudiobook: true}})
		ds.MediaFile().(*tests.MockMediaFileRepo).SetData(model.MediaFiles{
			{ID: "three", AlbumID: "book", DiscNumber: 1, TrackNumber: 3, Title: "Chapter 3"},
			{ID: "two", AlbumID: "book", DiscNumber: 1, TrackNumber: 2, Title: "Chapter 2"},
			{ID: "one", AlbumID: "book", DiscNumber: 1, TrackNumber: 1, Title: "Chapter 1"},
		})
		api := &Router{ds: ds}
		r := httptest.NewRequest("GET", "/rest/getAudiobook?id=book", nil).WithContext(context.Background())
		response, err := api.GetAudiobook(r)
		Expect(err).ToNot(HaveOccurred())
		Expect(response.Audiobook.Song).To(HaveLen(3))
		Expect(response.Audiobook.Song[0].Id).To(Equal("one"))
		Expect(response.Audiobook.Song[1].Id).To(Equal("two"))
		Expect(response.Audiobook.Song[2].Id).To(Equal("three"))
	})
})
