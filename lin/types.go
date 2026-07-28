package lin

import "github.com/SoundMatt/go-RCP/regmap"

// EndpointType re-exports regmap.EndpointTypeLIN so a caller that only
// imports this package doesn't also need to import server just to declare a
// LIN endpoint's type with server.Server.AddEndpoint.
const EndpointType = regmap.EndpointTypeLIN
