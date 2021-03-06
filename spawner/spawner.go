package spawner

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rumblefrog/Source-Map-Thumbnails/utils"

	"github.com/sirupsen/logrus"

	"github.com/rumblefrog/Source-Map-Thumbnails/config"
)

var (
	Command *exec.Cmd
)

// Function should be spawned in a separate goroutine, chan will notify if exited
func SpawnGame(terminate chan<- int8) {
	logrus.WithField("Game", config.Config.Game.Game).Info("Starting game")

	SpawnArgs := []string{
		"-steam",
		"-game " + config.Config.Game.Game,
		"-insecure",
		"-windowed",
		"-novid",
		"-usercon",
		"-ip " + utils.GetFirstLocalIPv4(), // Bind to a local interface so only we can connect
		"+map " + config.Config.Game.StartingMap,
		"+rcon_password smt", // A password required for rcon to start
	}

	// We are required to construct the CmdLine (Windows only) ourselves because hl2 cannot unquote the way golang quotes

	var cArg strings.Builder

	cArg.WriteString(filepath.Join(config.Config.Game.GameDirectory, config.Config.Game.EngineBinaryName))

	Command = exec.Command(cArg.String(), append(SpawnArgs, config.Config.Game.LaunchOptions...)...)

	cArg.WriteRune(' ')
	for _, v := range SpawnArgs {
		cArg.WriteRune(' ')
		cArg.WriteString(v)
	}

	for _, v := range config.Config.Game.LaunchOptions {
		cArg.WriteRune(' ')
		cArg.WriteString(v)
	}

	// The following two lines won't be needed if not on linux
	Command.SysProcAttr = &syscall.SysProcAttr{}

	Command.SysProcAttr.CmdLine = cArg.String()

	err := Command.Run()

	if err != nil {
		logrus.Info(err)

		terminate <- 0
	}

	Command.Wait()

	terminate <- 0
}
