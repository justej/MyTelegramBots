package timezone

import (
	"bufio"
	"errors"
	"fmt"
	"os"
)

var timeZones []Zone
var emptyZoneListError = errors.New("empty zone list")

type Zone struct {
	Code string
	*GeoLocation
	TZ string
}

func Init() error {
	z, err := LoadZonesFromFile("bots/FindingMemo/data/zone1970.tab")
	if err != nil {
		return err
	}

	timeZones = z
	return nil
}

// LoadZonesFromFile reads local zones*.tab file, parses it and returns parsed data as
// a slice of Zone.
// If parsing is impossible it returns already parsed time zones and the error.
func LoadZonesFromFile(name string) ([]Zone, error) {
	var zones []Zone

	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		var code, coord, tz string
		line := scanner.Text()
		if line[0] == '#' {
			continue
		}

		_, err := fmt.Sscanf(line, "%s\t%s\t%s", &code, &coord, &tz)
		if err != nil {
			return zones, err
		}

		lat, long, err := parseCoords(coord) // coordinates in degrees
		if err != nil {
			return zones, err
		}

		zones = append(zones, Zone{code, &GeoLocation{DegToRad(lat), DegToRad(long)}, tz})
	}

	return zones, nil
}

// parseCoords parses latitude and longitude according to the format in zones*.tab and
// returns them in degrees with fractional part.
func parseCoords(coords string) (lat float32, long float32, err error) {
	l := make([]float32, 6)
	s := make([]byte, 2)

	if len(coords) == 11 {
		// format ±DDMM±DDMM
		_, err = fmt.Sscanf(coords, "%c%2f%2f%c%3f%2f", &s[0], &l[0], &l[1], &s[1], &l[3], &l[4])
	} else {
		// format ±DDMMSS±DDMMSS
		_, err = fmt.Sscanf(coords, "%c%2f%2f%2f%c%3f%2f%2f", &s[0], &l[0], &l[1], &l[2], &s[1], &l[3], &l[4], &l[5])
		lat = l[2] / 3600
		long = l[5] / 3600
	}

	lat += l[0] + l[1]/60
	long += l[3] + l[4]/60

	if s[0] == '-' {
		lat = -lat
	}

	if s[1] == '-' {
		long = -long
	}

	return
}

// FindZone returns time zone of the nearest zone identifier
func (l *GeoLocation) FindZone() (*Zone, error) {
	if len(timeZones) == 0 {
		return nil, emptyZoneListError
	}

	if len(timeZones) == 1 {
		return &timeZones[0], nil
	}

	minDist := l.GreatCircleDistance(timeZones[0].GeoLocation)
	minIdx := 0
	for i, z := range timeZones[1:] {
		dist := l.GreatCircleDistance(z.GeoLocation)
		if minDist > dist {
			minDist = dist
			minIdx = i + 1
		}
	}

	return &timeZones[minIdx], nil
}
