package nativeapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMusicUploadPaths(t *testing.T) {
	for _, raw := range []string{"../outside.mp3", "album/../outside.mp3", "/outside.mp3", "album\\song.mp3", "album//song.mp3"} {
		if _, err := validUploadPath(raw); err == nil {
			t.Errorf("unsafe path %q was accepted", raw)
		}
	}
	if _, err := validUploadPath("Album/Disc 1/song.mp3"); err != nil {
		t.Fatal(err)
	}
	if safe, err := safeMusicComponent("AC/DC"); err != nil || safe != "AC-DC" {
		t.Fatalf("unexpected directory name %q, %v", safe, err)
	}

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "Artist")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ensureUploadDirectory(root, "Artist", "Album"); err == nil {
		t.Fatal("an upload followed a symlink out of the library")
	}
}

func TestMusicUploadCompanionAssignment(t *testing.T) {
	files := []string{"A/Album/cover.jpg", "A/Album/Disc 1/one.mp3", "A/Album/Disc 2/two.mp3"}
	assignments := map[string]uploadedTrack{
		files[1]: {Album: "Album", AlbumArtist: "A"},
		files[2]: {Album: "Album", AlbumArtist: "A"},
	}
	if got := assignUploadCompanions(files, assignments)[files[0]]; got.Album != "Album" {
		t.Errorf("cover not assigned to tracks in disc folders: %+v", got)
	}
	assignments[files[2]] = uploadedTrack{Album: "Another Album", AlbumArtist: "A"}
	if _, ok := assignUploadCompanions(files, assignments)[files[0]]; ok {
		t.Fatal("ambiguous cover assigned to the wrong album")
	}
}

func TestMusicUploadCompilationAcrossDiscs(t *testing.T) {
	if uploadDiscFolder("Album/Disc 1/01.mp3") != "Disc 1" || uploadDiscFolder("Album/Disc 2/01.mp3") != "Disc 2" {
		t.Fatal("disc folders were not preserved with their track filenames")
	}
	tracks := []uploadedTrack{
		{Path: "Collection/Disc 1/one.mp3", Album: "Collection", Artist: "Artist One"},
		{Path: "Collection/Disc 2/two.mp3", Album: "Collection", Artist: "Artist Two"},
	}
	groupCompilations(tracks)
	for _, track := range tracks {
		if track.AlbumArtist != "Various Artists" {
			t.Fatalf("expected compilation artist for %s, got %q", track.Path, track.AlbumArtist)
		}
	}
}
