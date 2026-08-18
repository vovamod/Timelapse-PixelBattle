package entities

type GPU struct {
	Name         string
	Vendor       string
	IsIntegrated bool
	Driver       string
	PCIAddress   string // Linux only (press F for others)
}

type GPUSelection struct {
	Encoder     string
	EncoderName string
	GPUType     string
	Device      string
}
