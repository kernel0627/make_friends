package api

import (
	"testing"

	"make_friends/backend/internal/model"
)

func TestLocationProximityScorePrefersNearbyPosts(t *testing.T) {
	viewer := &geoPoint{Latitude: 40.1572, Longitude: 116.2875}
	nearPost := model.Post{Lat: 40.1574, Lng: 116.2878}
	farPost := model.Post{Lat: 31.2304, Lng: 121.4737}

	nearScore := locationProximityScore(viewer, nearPost)
	farScore := locationProximityScore(viewer, farPost)

	if nearScore <= farScore {
		t.Fatalf("near post should score higher than far post, near=%f far=%f", nearScore, farScore)
	}
	if nearScore < 0.9 {
		t.Fatalf("near post should receive a strong proximity boost, got %f", nearScore)
	}
	if farScore != 0 {
		t.Fatalf("far post beyond city range should not receive proximity boost, got %f", farScore)
	}
}

func TestLocationProximityScoreIgnoresMissingPostCoords(t *testing.T) {
	viewer := &geoPoint{Latitude: 40.1572, Longitude: 116.2875}
	postWithoutCoords := model.Post{}

	if score := locationProximityScore(viewer, postWithoutCoords); score != 0 {
		t.Fatalf("missing post coords should not receive proximity boost, got %f", score)
	}
}
