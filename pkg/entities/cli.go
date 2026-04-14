package entities

type CLI struct {
	Render struct {
		Output string `help:"Output video file" required:""`
	} `cmd:"" help:"Render video"`

	Photo struct {
		Output string `help:"Output image file" required:""`
	} `cmd:"" help:"Generate photo"`

	Width       int    `default:"1080"`
	Height      int    `default:"1920"`
	Iterations  int    `default:"16"`
	TextureSize int    `name:"texture-size" default:"16"`
	Framerate   int    `default:"24"`
	PlayerName  string `name:"playername"`

	DBSource   string `name:"db-source"`
	DBIp       string `name:"db-ip"`
	DBUser     string `name:"db-user"`
	DBPassword string `name:"db-password"`
	DBName     string `name:"db-name"`
	DBTable    string `name:"db-table"`
	DBTLS      bool   `name:"db-tls"`

	Local    bool
	WithInfo bool `name:"with-info"`
	Debug    bool
}
