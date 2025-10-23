package log

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

var Logger = logrus.New()

func Init()(*os.File, error) {
	// Logger.SetOutput(os.Stdout)

	logFileName := fmt.Sprintf("logs/run-%s.log", time.Now().Format("2006-01-02-15-04-05"))
	
	// Create the 'logs' directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		Logger.WithError(err).Error("Could not create logs directory")
		return nil, err
	}

	// Open log file
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		Logger.WithError(err).Error("Failed to open log file")
		return nil, err
	}

	// log to both stdout and the file
	mw := io.MultiWriter(os.Stdout, logFile)
	Logger.SetOutput(mw)


	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	level := logrus.InfoLevel 

	Logger.SetLevel(level)

	// set to true if you want the location of log (debugging)
	Logger.SetReportCaller(false)
	return logFile, nil
}