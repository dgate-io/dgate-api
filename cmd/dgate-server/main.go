package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"runtime/debug"

	"github.com/dgate-io/dgate-api/internal/admin"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/spf13/pflag"
)

var version string = "dev"

func main() {
	showVersion := pflag.BoolP("version", "v", false, "print current version")
	configPath := pflag.StringP("config", "c", "", "path to config file")
	help := pflag.BoolP("help", "h", false, "show help")

	pflag.Parse()

	if *help {
		pflag.Usage()
		return
	}

	// get version from build info when installed using `go install``
	buildInfo, ok := debug.ReadBuildInfo()
	if ok && buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)" {
		version = buildInfo.Main.Version
	}

	if version == "dev" {
		fmt.Printf("PID:%d\n", os.Getpid())
		fmt.Printf("GOMAXPROCS:%d\n", runtime.GOMAXPROCS(0))
	}

	if *showVersion {
		println(version)
	} else {
		if !util.EnvVarCheckBool("DG_DISABLE_BANNER") {
			fmt.Println(
				"_________________      _____      \n" +
					"___  __ \\_  ____/_____ __  /_____ \n" +
					"__  / / /  / __ _  __ `/  __/  _ \\\n" +
					"_  /_/ // /_/ / / /_/ // /_ /  __/\n" +
					"/_____/ \\____/  \\__,_/ \\__/ \\___/ \n" +
					"                                   \n" +
					"DGate - API Gateway Server (" + version + ")\n" +
					"-----------------------------------\n",
			)
		}
		if dgateConfig, err := config.LoadConfig(*configPath); err != nil {
			fmt.Printf("Error loading config: %s\n", err)
			panic(err)
		} else {
			logger, err := dgateConfig.GetLogger()
			if err != nil {
				fmt.Printf("Error setting up logger: %s\n", err)
				panic(err)
			}
			defer logger.Sync()
			proxyState := proxy.NewProxyState(logger.Named("proxy"), dgateConfig)
			err = admin.StartAdminAPI(version, dgateConfig, logger.Named("admin"), proxyState)
			if err != nil {
				fmt.Printf("Error starting admin api: %s\n", err)
				panic(err)
			}
			if err := proxyState.Start(); err != nil {
				fmt.Printf("Error loading config: %s\n", err)
				panic(err)
			}

			sigchan := make(chan os.Signal, 1)
			signal.Notify(sigchan,
				syscall.SIGINT,
				syscall.SIGTERM,
				syscall.SIGQUIT,
			)
			<-sigchan
			proxyState.Stop()
		}
	}
}
