package dot

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// parseColour reads a DOT colour: "#rrggbb" or "#rrggbbaa", "#rgb", or a
// name of the X11 scheme Graphviz uses by default -- the common ones; a
// list, "red:blue", is its first colour. A name not known here is an error
// naming it, rather than a colour quietly drawn as some other.
func parseColour(v string) (color.Color, error) {
	v = strings.TrimSpace(v)
	if i := strings.IndexAny(v, ":;"); i >= 0 {
		v = v[:i]
	}
	if strings.HasPrefix(v, "#") {
		hex := v[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) != 6 && len(hex) != 8 {
			return nil, fmt.Errorf("a colour is #rrggbb or #rrggbbaa")
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("a colour is #rrggbb or #rrggbbaa")
		}
		if len(hex) == 6 {
			return color.RGBA{uint8(n >> 16), uint8(n >> 8), uint8(n), 0xFF}, nil
		}
		return color.NRGBA{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
	}
	name := strings.TrimPrefix(strings.ToLower(v), "/x11/")
	if name == "transparent" || name == "none" {
		return color.NRGBA{}, nil
	}
	if c, ok := colourNames[name]; ok {
		return color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 0xFF}, nil
	}
	return nil, fmt.Errorf("no colour %q is known here: use #rrggbb, or a common name such as red, darkgreen or lightgrey", v)
}

// colourNames are the X11 colours a diagram is most often coloured in.
var colourNames = map[string]uint32{
	"black": 0x000000, "white": 0xFFFFFF, "red": 0xFF0000, "green": 0x00FF00, "blue": 0x0000FF,
	"yellow": 0xFFFF00, "cyan": 0x00FFFF, "magenta": 0xFF00FF, "orange": 0xFFA500, "purple": 0xA020F0,
	"brown": 0xA52A2A, "pink": 0xFFC0CB, "gold": 0xFFD700, "navy": 0x000080, "teal": 0x008080,
	"gray": 0xC0C0C0, "grey": 0xC0C0C0, "lightgray": 0xD3D3D3, "lightgrey": 0xD3D3D3,
	"darkgray": 0xA9A9A9, "darkgrey": 0xA9A9A9, "dimgray": 0x696969, "dimgrey": 0x696969,
	"lightblue": 0xADD8E6, "lightgreen": 0x90EE90, "lightyellow": 0xFFFFE0, "lightpink": 0xFFB6C1,
	"darkgreen": 0x006400, "darkred": 0x8B0000, "darkblue": 0x00008B, "darkorange": 0xFF8C00,
	"forestgreen": 0x228B22, "limegreen": 0x32CD32, "seagreen": 0x2E8B57, "crimson": 0xDC143C,
	"firebrick": 0xB22222, "tomato": 0xFF6347, "salmon": 0xFA8072, "steelblue": 0x4682B4,
	"royalblue": 0x4169E1, "skyblue": 0x87CEEB, "dodgerblue": 0x1E90FF, "slategray": 0x708090,
	"slategrey": 0x708090, "khaki": 0xF0E68C, "beige": 0xF5F5DC, "ivory": 0xFFFFF0, "lavender": 0xE6E6FA,
	"violet": 0xEE82EE, "orchid": 0xDA70D6, "goldenrod": 0xDAA520, "chocolate": 0xD2691E, "tan": 0xD2B48C,
	"coral": 0xFF7F50, "turquoise": 0x40E0D0, "aquamarine": 0x7FFFD4, "olivedrab": 0x6B8E23,
	"palegreen": 0x98FB98, "honeydew": 0xF0FFF0, "mintcream": 0xF5FFFA, "aliceblue": 0xF0F8FF,
	"whitesmoke": 0xF5F5F5, "gainsboro": 0xDCDCDC, "silver": 0xC0C0C0, "maroon": 0xB03060,
	"olive": 0x808000, "indigo": 0x4B0082,
}
