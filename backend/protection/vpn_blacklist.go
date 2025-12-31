package protection

// GetVPNExecutables returns a list of common VPN process names to block
func GetVPNExecutables() []string {
	return []string{
		"NordVPN.exe",
		"ExpressVPN.exe",
		"openvpn.exe",
		"wireguard.exe",
		"pia-client.exe",
		"Surfshark.exe",
		"CyberGhost.exe",
		"ProtonVPN.exe",
		"Windscribe.exe",
		"Mullvad VPN.exe",
		"HotspotShield.exe",
		"TunnelBear.exe",
		"avgvpn.exe",
		"vpndaemon.exe",
		// Add more as needed
	}
}

// GetVPNDomains returns a list of common VPN websites to block
func GetVPNDomains() []string {
	return []string{
		"nordvpn.com",
		"expressvpn.com",
		"openvpn.net",
		"wireguard.com",
		"privateinternetaccess.com",
		"surfshark.com",
		"cyberghostvpn.com",
		"protonvpn.com",
		"windscribe.com",
		"mullvad.net",
		"hotspotshield.com",
		"tunnelbear.com",
		"avg.com", // Risky if they use AVG antivirus
		// "avast.com", // Risky
	}
}
