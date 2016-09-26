package localdriver_test

import (
	"errors"
	"fmt"
	"os"
	"path"

	"code.cloudfoundry.org/goshims/filepath/filepath_fake"
	"code.cloudfoundry.org/goshims/os/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/localdriver"
	"code.cloudfoundry.org/voldriver"
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Local Driver", func() {
	var logger lager.Logger
	var ctx context.Context
	var fakeOs *os_fake.FakeOs
	var fakeFilepath *filepath_fake.FakeFilepath
	var localDriver *localdriver.LocalDriver
	var mountDir string
	var volumeId string

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("localdriver-local")
		ctx = context.TODO()

		mountDir = "/path/to/mount"

		fakeOs = &os_fake.FakeOs{}
		fakeFilepath = &filepath_fake.FakeFilepath{}
		localDriver = localdriver.NewLocalDriver(fakeOs, fakeFilepath, mountDir)
		volumeId = "test-volume-id"
	})

	Describe("#Activate", func() {
		It("returns Implements: VolumeDriver", func() {
			activateResponse := localDriver.Activate(logger)
			Expect(len(activateResponse.Implements)).To(BeNumerically(">", 0))
			Expect(activateResponse.Implements[0]).To(Equal("VolumeDriver"))
		})
	})

	Describe("Mount", func() {

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
				mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, "")
			})

			AfterEach(func() {
				unmountSuccessful(logger, localDriver, volumeId)
				removeSuccessful(logger, localDriver, volumeId)
			})

			It("should mount the volume on the local filesystem", func() {
				Expect(fakeFilepath.AbsCallCount()).To(Equal(3))
				Expect(fakeOs.MkdirAllCallCount()).To(Equal(4))
				Expect(fakeOs.SymlinkCallCount()).To(Equal(1))
				from, to := fakeOs.SymlinkArgsForCall(0)
				Expect(from).To(Equal("/path/to/mount/_volumes/test-volume-id"))
				Expect(to).To(Equal("/path/to/mount/_mounts/test-volume-id"))
			})

			It("returns the mount point on a /VolumeDriver.Get response", func() {
				getResponse := getSuccessful(logger, localDriver, volumeId)
				Expect(getResponse.Volume.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
			})
		})

		Context("when the volume has been created with a passcode", func() {
			const passcode = "aPassc0de"

			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, passcode)
			})

			AfterEach(func() {
				removeSuccessful(logger, localDriver, volumeId)
			})

			Context("when mounting with the right passcode", func() {
				BeforeEach(func() {
					mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, passcode)
				})
				AfterEach(func() {
					unmountSuccessful(logger, localDriver, volumeId)
				})

				It("should mount the volume on the local filesystem", func() {
					Expect(fakeFilepath.AbsCallCount()).To(Equal(3))
					Expect(fakeOs.MkdirAllCallCount()).To(Equal(4))
					Expect(fakeOs.SymlinkCallCount()).To(Equal(1))
					from, to := fakeOs.SymlinkArgsForCall(0)
					Expect(from).To(Equal("/path/to/mount/_volumes/test-volume-id"))
					Expect(to).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})

				It("returns the mount point on a /VolumeDriver.Get response", func() {
					getResponse := getSuccessful(logger, localDriver, volumeId)
					Expect(getResponse.Volume.Mountpoint).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})
			})

			Context("when mounting with the wrong passcode", func() {
				It("returns an error", func() {
					mountResponse := localDriver.Mount(logger, ctx, voldriver.MountRequest{
						Name: volumeId,
						Opts: map[string]interface{}{"passcode": "wrong"},
					})
					Expect(mountResponse.Err).To(Equal("Volume " + volumeId + " access denied"))
				})
			})

			Context("when mounting with the wrong passcode type", func() {
				It("returns an error", func() {
					mountResponse := localDriver.Mount(logger, ctx, voldriver.MountRequest{
						Name: volumeId,
						Opts: map[string]interface{}{"passcode": nil},
					})
					Expect(mountResponse.Err).To(Equal("Opts.passcode must be a string value"))
				})
			})

			Context("when mounting with no passcode", func() {
				It("returns an error", func() {
					mountResponse := localDriver.Mount(logger, ctx, voldriver.MountRequest{
						Name: volumeId,
					})
					Expect(mountResponse.Err).To(Equal("Volume " + volumeId + " requires a passcode"))
				})
			})

		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				mountResponse := localDriver.Mount(logger, ctx, voldriver.MountRequest{
					Name: "bla",
				})
				Expect(mountResponse.Err).To(Equal("Volume 'bla' must be created before being mounted"))
			})
		})
	})

	Describe("Unmount", func() {
		Context("when a volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
			})

			Context("when a volume has been mounted", func() {
				BeforeEach(func() {
					mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, "")
				})

				It("After unmounting /VolumeDriver.Get returns no mountpoint", func() {
					unmountSuccessful(logger, localDriver, volumeId)
					getResponse := getSuccessful(logger, localDriver, volumeId)
					Expect(getResponse.Volume.Mountpoint).To(Equal(""))
				})

				It("/VolumeDriver.Unmount doesn't remove mountpath from OS", func() {
					unmountSuccessful(logger, localDriver, volumeId)
					Expect(fakeOs.RemoveCallCount()).To(Equal(1))
					removed := fakeOs.RemoveArgsForCall(0)
					Expect(removed).To(Equal("/path/to/mount/_mounts/test-volume-id"))
				})

				Context("when the same volume is mounted a second time then unmounted", func() {
					BeforeEach(func() {
						mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, "")
						unmountSuccessful(logger, localDriver, volumeId)
					})

					It("should not report empty mountpoint until unmount is called again", func() {
						getResponse := getSuccessful(logger, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))

						unmountSuccessful(logger, localDriver, volumeId)
						getResponse = getSuccessful(logger, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).To(Equal(""))
					})
				})
				Context("when the mountpath is not found on the filesystem", func() {
					var unmountResponse voldriver.ErrorResponse

					BeforeEach(func() {
						fakeOs.StatReturns(nil, os.ErrNotExist)
						unmountResponse = localDriver.Unmount(logger, voldriver.UnmountRequest{
							Name: volumeId,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(Equal("Volume " + volumeId + " does not exist (path: /path/to/mount/_mounts/test-volume-id), nothing to do!"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(logger, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})

				Context("when the mountpath cannot be accessed", func() {
					var unmountResponse voldriver.ErrorResponse

					BeforeEach(func() {
						fakeOs.StatReturns(nil, errors.New("something weird"))
						unmountResponse = localDriver.Unmount(logger, voldriver.UnmountRequest{
							Name: volumeId,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(Equal("Error establishing whether volume exists"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(logger, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})
			})

			Context("when the volume has not been mounted", func() {
				It("returns an error", func() {
					unmountResponse := localDriver.Unmount(logger, voldriver.UnmountRequest{
						Name: volumeId,
					})

					Expect(unmountResponse.Err).To(Equal("Volume not previously mounted"))
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				unmountResponse := localDriver.Unmount(logger, voldriver.UnmountRequest{
					Name: volumeId,
				})

				Expect(unmountResponse.Err).To(Equal(fmt.Sprintf("Volume '%s' not found", volumeId)))
			})
		})
	})

	Describe("Create", func() {
		Context("when a passcode is wrong type", func() {
			It("returns an error", func() {
				createResponse := localDriver.Create(logger, voldriver.CreateRequest{
					Name: "volume",
					Opts: map[string]interface{}{
						"passcode": nil,
					},
				})

				Expect(createResponse.Err).To(Equal("Opts.passcode must be a string value"))
			})
		})

		Context("when a second create is called with the same volume ID", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, "volume", "")
			})

			Context("with the same opts", func() {
				It("does nothing", func() {
					createSuccessful(logger, localDriver, fakeOs, "volume", "")
				})
			})
		})
	})

	Describe("Get", func() {
		Context("when the volume has been created", func() {
			It("returns the volume name", func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
				getSuccessful(logger, localDriver, volumeId)
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				getUnsuccessful(logger, localDriver, volumeId)
			})
		})
	})

	Describe("Path", func() {
		Context("when a volume is mounted", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
				mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, "")
			})

			It("returns the mount point on a /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(logger, voldriver.PathRequest{
					Name: volumeId,
				})
				Expect(pathResponse.Err).To(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/" + volumeId))
			})
		})

		Context("when a volume is not created", func() {
			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(logger, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})

		Context("when a volume is created but not mounted", func() {
			var (
				volumeName string
			)
			BeforeEach(func() {
				volumeName = "my-volume"
				createSuccessful(logger, localDriver, fakeOs, volumeName, "")
			})

			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(logger, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})
	})

	Describe("List", func() {
		Context("when there are volumes", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
			})

			It("returns the list of volumes", func() {
				listResponse := localDriver.List(logger)

				Expect(listResponse.Err).To(Equal(""))
				Expect(listResponse.Volumes[0].Name).To(Equal(volumeId))

			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				volumeName := "test-volume-2"
				getUnsuccessful(logger, localDriver, volumeName)
			})
		})
	})

	Describe("Remove", func() {
		It("should fail if no volume name provided", func() {
			removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
				Name: "",
			})
			Expect(removeResponse.Err).To(Equal("Missing mandatory 'volume_name'"))
		})

		It("should fail if no volume was created", func() {
			removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
				Name: volumeId,
			})
			Expect(removeResponse.Err).To(Equal("Volume '" + volumeId + "' not found"))
		})

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(logger, localDriver, fakeOs, volumeId, "")
			})

			It("/VolumePlugin.Remove destroys volume", func() {
				removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeId,
				})
				Expect(removeResponse.Err).To(Equal(""))
				Expect(fakeOs.RemoveAllCallCount()).To(Equal(1))

				getUnsuccessful(logger, localDriver, volumeId)
			})

			Context("when volume has been mounted", func() {
				It("/VolumePlugin.Remove unmounts and destroys volume", func() {
					mountSuccessful(logger, ctx, localDriver, volumeId, fakeFilepath, "")

					removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
						Name: volumeId,
					})
					Expect(removeResponse.Err).To(Equal(""))
					Expect(fakeOs.RemoveCallCount()).To(Equal(1))
					Expect(fakeOs.RemoveAllCallCount()).To(Equal(1))

					getUnsuccessful(logger, localDriver, volumeId)
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
					Name: volumeId,
				})
				Expect(removeResponse.Err).To(Equal("Volume '" + volumeId + "' not found"))
			})
		})
	})
})

func getUnsuccessful(logger lager.Logger, localDriver voldriver.Driver, volumeName string) {
	getResponse := localDriver.Get(logger, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal("Volume not found"))
	Expect(getResponse.Volume.Name).To(Equal(""))
}

func getSuccessful(logger lager.Logger, localDriver voldriver.Driver, volumeName string) voldriver.GetResponse {
	getResponse := localDriver.Get(logger, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal(""))
	Expect(getResponse.Volume.Name).To(Equal(volumeName))
	return getResponse
}

func createSuccessful(logger lager.Logger, localDriver voldriver.Driver, fakeOs *os_fake.FakeOs, volumeName string, passcode string) {
	opts := map[string]interface{}{}
	if passcode != "" {
		opts["passcode"] = passcode
	}
	createResponse := localDriver.Create(logger, voldriver.CreateRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(createResponse.Err).To(Equal(""))

	Expect(fakeOs.MkdirAllCallCount()).Should(Equal(2))

	volumeDir, fileMode := fakeOs.MkdirAllArgsForCall(1)
	Expect(path.Base(volumeDir)).To(Equal(volumeName))
	Expect(fileMode).To(Equal(os.ModePerm))
}

func mountSuccessful(logger lager.Logger, ctx context.Context, localDriver voldriver.Driver, volumeName string, fakeFilepath *filepath_fake.FakeFilepath, passcode string) {
	fakeFilepath.AbsReturns("/path/to/mount/", nil)
	opts := map[string]interface{}{}
	if passcode != "" {
		opts["passcode"] = passcode
	}
	mountResponse := localDriver.Mount(logger, ctx, voldriver.MountRequest{
		Name: volumeName,
		Opts: opts,
	})
	Expect(mountResponse.Err).To(Equal(""))
	Expect(mountResponse.Mountpoint).To(Equal("/path/to/mount/_mounts/" + volumeName))
}

func unmountSuccessful(logger lager.Logger, localDriver voldriver.Driver, volumeName string) {
	unmountResponse := localDriver.Unmount(logger, voldriver.UnmountRequest{
		Name: volumeName,
	})
	Expect(unmountResponse.Err).To(Equal(""))
}

func removeSuccessful(logger lager.Logger, localDriver voldriver.Driver, volumeName string) {
	removeResponse := localDriver.Remove(logger, voldriver.RemoveRequest{
		Name: volumeName,
	})
	Expect(removeResponse.Err).To(Equal(""))
}
