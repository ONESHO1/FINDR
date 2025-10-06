package log

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Logger = logrus.New()

func Init() {
	Logger.SetOutput(os.Stdout)


	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	level := logrus.InfoLevel 

	Logger.SetLevel(level)

	// set to true if you want the location of log (debugging)
	Logger.SetReportCaller(false)
}