package main_test

import (
	"io/ioutil"
	"net"
	"os/exec"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Main", func() {
	var (
		session *gexec.Session
		command *exec.Cmd
		err     error
		logger  lager.Logger
	)

	BeforeEach(func() {
		command = exec.Command(driverPath)
		logger = lagertest.NewTestLogger("test-localdriver")
	})

	JustBeforeEach(func() {
		session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		session.Kill().Wait()
	})

	Context("with a driver path", func() {
		BeforeEach(func() {
			dir, err := ioutil.TempDir("", "driversPath")
			Expect(err).ToNot(HaveOccurred())

			command.Args = append(command.Args, "-driversPath="+dir)
		})

		It("listens on tcp/9750 by default", func() {
			EventuallyWithOffset(1, func() error {
				_, err := net.Dial("tcp", "0.0.0.0:9750")
				return err
			}, 5).ShouldNot(HaveOccurred())
		})

		Context("in another context", func() {
			BeforeEach(func() {
				command.Args = append(command.Args, "-listenAddr=0.0.0.0:9751")
			})

			It("listens on tcp/9751", func() {

				EventuallyWithOffset(1, func() error {
					_, err := net.Dial("tcp", "0.0.0.0:9751")
					return err
				}, 5).ShouldNot(HaveOccurred())
			})
		})

	})
})
