package ntp

import (
	"fmt"
	"time"

	"github.com/beevik/ntp"
)

// GetNetworkTime attempts to fetch the current time from an NTP server.
// It returns the current time and any error encountered.
func GetNetworkTime() (time.Time, error) {
	// Try a few reliable pools
	servers := []string{
		"pool.ntp.org",
		"time.google.com",
		"time.windows.com",
	}

	for _, server := range servers {
		t, err := ntp.Time(server)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("all NTP servers failed")
}

// GetOffset returns the difference between Network Time and System Time.
// offset > 0 means Network Time is ahead (System Time is behind).
func GetOffset() (time.Duration, error) {
	ntpTime, err := GetNetworkTime()
	if err != nil {
		return 0, err
	}
	return ntpTime.Sub(time.Now()), nil
}
