package audio

import "strings"

func GetFFmpegFilterArgs(filterName string) []string {
	filterName = strings.ToLower(strings.TrimSpace(filterName))
	var afFilter string

	switch filterName {
	case "bassboost", "bass":
		afFilter = "equalizer=f=60:width_type=h:width=50:g=10"
	case "nightcore":
		afFilter = "asetrate=48000*1.25,aresample=48000,atempo=1.05"
	case "vaporwave":
		afFilter = "asetrate=48000*0.8,aresample=48000,atempo=0.9"
	case "8d":
		afFilter = "apulsator=hz=0.125"
	case "pop":
		afFilter = "equalizer=f=1000:width_type=h:width=200:g=5"
	default:
		return nil
	}

	return []string{"-af", afFilter}
}
