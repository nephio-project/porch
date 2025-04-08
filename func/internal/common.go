package internal

import (
	"flag"
	"k8s.io/klog/v2"
	"strconv"
)

func SetLogLevel(l int) {
	if l > 5 {
		l = 5
	}
	flagSet := flag.NewFlagSet("log-level", flag.ContinueOnError)
	klog.InitFlags(flagSet)
	_ = flagSet.Parse([]string{"--v", strconv.Itoa(l)})
}
