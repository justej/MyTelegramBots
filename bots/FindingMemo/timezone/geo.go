package timezone

import "math"

type GeoLocation struct {
	Latitude  float32 // in radians
	Longitude float32 // in radians
}

const R float64 = 6371e3 // radius of the Earth in metres

// GreatCircleDistance computes Haversine distance between two points in meters
// See http://www.movable-type.co.uk/scripts/latlong.html
func (l *GeoLocation) GreatCircleDistance(loc *GeoLocation) float32 {
	lat1 := float64(l.Latitude)
	lat2 := float64(loc.Latitude)
	dLat := lat2 - lat1
	dLon := float64(l.Longitude - loc.Longitude)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return float32(R * c)
}

func DegToRad(deg float32) float32 {
	return deg * math.Pi / 180
}
