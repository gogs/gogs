package pam

// Config contains configuration for PAM authentication.
//
// ⚠️ WARNING: Change to the field name must preserve the INI key name for backward compatibility.
type Config struct {
	// The name of the PAM service, e.g. system-auth.
	ServiceName string
}
