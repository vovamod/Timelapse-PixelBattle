package entities

type GPU struct {
	Name         string
	Vendor       string
	IsIntegrated bool
	Driver       string
}

type GPUSelection struct {
	Encoder     string
	EncoderName string
	GPUType     string
}
