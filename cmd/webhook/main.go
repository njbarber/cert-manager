/*
Copyright 2019 The Jetstack cert-manager contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/klog"
	"k8s.io/klog/klogr"

	"github.com/jetstack/cert-manager/pkg/logs"
	"github.com/jetstack/cert-manager/pkg/webhook"
	"github.com/jetstack/cert-manager/pkg/webhook/handlers"
	"github.com/jetstack/cert-manager/pkg/webhook/server"
)

var (
	securePort  int
	healthzPort int
	tlsCertFile string
	tlsKeyFile  string
)

func init() {
	flag.IntVar(&healthzPort, "healthz-port", 6080, "port number to listen on for insecure healthz connections")
	flag.IntVar(&securePort, "secure-port", 6443, "port number to listen on for secure TLS connections")
	flag.StringVar(&tlsCertFile, "tls-cert-file", "", "path to the file containing the TLS certificate to serve with")
	flag.StringVar(&tlsKeyFile, "tls-private-key-file", "", "path to the file containing the TLS private key to serve with")
}

var validationHook handlers.ValidatingAdmissionHook = handlers.NewFuncBackedValidator(logs.Log, webhook.Scheme, webhook.Validators)
var mutationHook handlers.MutatingAdmissionHook = handlers.NewSchemeBackedDefaulter(logs.Log, webhook.Scheme)
var conversionHook handlers.ConversionHook = handlers.NewSchemeBackedConverter(logs.Log, webhook.Scheme)

func main() {
	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	log := klogr.New()
	stopCh := setupSignalHandler()

	var source server.CertificateSource
	if tlsCertFile == "" || tlsKeyFile == "" {
		log.Info("warning: serving insecurely as tls certificate data not provided")
	} else {
		log.Info("enabling TLS as certificate file flags specified")
		source = &server.FileCertificateSource{
			CertPath: tlsCertFile,
			KeyPath:  tlsKeyFile,
			Log:      log,
		}
	}

	srv := server.Server{
		ListenAddr:        fmt.Sprintf(":%d", securePort),
		HealthzAddr:       fmt.Sprintf(":%d", healthzPort),
		EnablePprof:       true,
		CertificateSource: source,
		ValidationWebhook: validationHook,
		MutationWebhook:   mutationHook,
		ConversionWebhook: conversionHook,
		Log:               log,
	}
	if err := srv.Run(stopCh); err != nil {
		log.Error(err, "error running server")
		os.Exit(1)
	}
}

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
var onlyOneSignalHandler = make(chan struct{})

// setupSignalHandler registered for SIGTERM and SIGINT. A stop channel is returned
// which is closed on one of these signals. If a second signal is caught, the program
// is terminated with exit code 1.
func setupSignalHandler() (stopCh <-chan struct{}) {
	close(onlyOneSignalHandler) // panics when called twice

	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1) // second signal. Exit directly.
	}()

	return stop
}
