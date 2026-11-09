package panel

// Who gives way when the server runs out of memory: the kernel stops the highest score first. Skiff's router and dashboard sit far below all of these, set in their system services.
const (
	oomProduction = 300
	oomOneOff     = 500 // jobs and one-off runs: they can be run again
	oomPreview    = 600
)

func oomScoreFor(src Source) int {
	if src.Parent != "" {
		return oomPreview
	}
	return oomProduction
}
