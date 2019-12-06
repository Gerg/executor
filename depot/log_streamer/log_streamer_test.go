package log_streamer_test

import (
	"context"
	"fmt"
	"strings"
	"sync"

	mfakes "code.cloudfoundry.org/diego-logging-client/testhelpers"
	"code.cloudfoundry.org/executor/depot/log_streamer"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogStreamer", func() {
	var (
		streamer   log_streamer.LogStreamer
		fakeClient *mfakes.FakeIngressClient
		ctx        context.Context
		cancelFunc context.CancelFunc
	)

	guid := "the-guid"
	sourceName := "the-source-name"
	index := 11
	tags := map[string]string{
		"foo": "bar",
		"biz": "baz",
	}

	BeforeEach(func() {
		ctx, cancelFunc = context.WithCancel(context.Background())
		fakeClient = &mfakes.FakeIngressClient{}
		streamer = log_streamer.New(ctx, guid, sourceName, index, tags, fakeClient)
	})

	Context("when told to emit", func() {
		Context("when given a message that corresponds to one line", func() {
			BeforeEach(func() {
				fmt.Fprintln(streamer.Stdout(), "this is a log")
				fmt.Fprintln(streamer.Stdout(), "this is another log")
			})

			It("should emit that message", func() {
				Expect(fakeClient.SendAppLogCallCount()).To(Equal(2))

				message, sn, tags := fakeClient.SendAppLogArgsForCall(0)
				Expect(tags["source_id"]).To(Equal(guid))
				Expect(sn).To(Equal(sourceName))
				Expect(message).To(Equal("this is a log"))
				Expect(tags["instance_id"]).To(Equal("11"))
				Expect(tags["foo"]).To(Equal("bar"))
				Expect(tags["biz"]).To(Equal("baz"))

				message, sn, tags = fakeClient.SendAppLogArgsForCall(1)
				Expect(tags["source_id"]).To(Equal(guid))
				Expect(sn).To(Equal(sourceName))
				Expect(message).To(Equal("this is another log"))
				Expect(tags["instance_id"]).To(Equal("11"))
				Expect(tags["foo"]).To(Equal("bar"))
				Expect(tags["biz"]).To(Equal("baz"))
			})
		})

		Describe("WithSource", func() {
			Context("when a new log source is provided", func() {
				It("should emit a message with the new log source", func() {
					newSourceName := "new-source-name"
					streamer = streamer.WithSource(newSourceName)
					fmt.Fprintln(streamer.Stdout(), "this is a log")
					Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))

					_, sn, _ := fakeClient.SendAppLogArgsForCall(0)

					Expect(sn).To(Equal(newSourceName))
				})
			})

			Context("when no log source is provided", func() {
				It("should emit a message with the existing log source", func() {
					streamer = streamer.WithSource("")
					fmt.Fprintln(streamer.Stdout(), "this is a log")

					Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))

					_, sn, _ := fakeClient.SendAppLogArgsForCall(0)

					Expect(sn).To(Equal(sourceName))
				})
			})
		})

		Describe("SourceName", func() {
			It("should return the log streamer's configured source name", func() {
				Expect(streamer.SourceName()).To(Equal(sourceName))
			})
		})

		Context("when given a message with all sorts of fun newline characters", func() {
			BeforeEach(func() {
				fmt.Fprintf(streamer.Stdout(), "A\nB\rC\n\rD\r\nE\n\n\nF\r\r\rG\n\r\r\n\n\n\r")
			})

			It("should do the right thing", func() {
				Expect(fakeClient.SendAppLogCallCount()).To(Equal(7))
				for i, expectedString := range []string{"A", "B", "C", "D", "E", "F", "G"} {
					message, _, _ := fakeClient.SendAppLogArgsForCall(i)
					Expect(message).To(Equal(expectedString))
				}
			})
		})

		Context("when given a series of short messages", func() {
			BeforeEach(func() {
				fmt.Fprintf(streamer.Stdout(), "this is a log")
				fmt.Fprintf(streamer.Stdout(), " it is made of wood")
				fmt.Fprintf(streamer.Stdout(), " - and it is longer")
				fmt.Fprintf(streamer.Stdout(), "than it seems\n")
			})

			It("concatenates them, until a new-line is received, and then emits that", func() {
				Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))
				message, _, _ := fakeClient.SendAppLogArgsForCall(0)
				Expect(message).To(Equal("this is a log it is made of wood - and it is longerthan it seems"))
			})
		})

		Context("when given a message with multiple new lines", func() {
			BeforeEach(func() {
				fmt.Fprintf(streamer.Stdout(), "this is a log\nand this is another\nand this one isn't done yet...")
			})

			It("should break the message up into multiple loggings", func() {
				Expect(fakeClient.SendAppLogCallCount()).To(Equal(2))

				message, _, _ := fakeClient.SendAppLogArgsForCall(0)
				Expect(message).To(Equal("this is a log"))

				message, _, _ = fakeClient.SendAppLogArgsForCall(1)
				Expect(message).To(Equal("and this is another"))
			})
		})

		Describe("message limits", func() {
			var message string
			Context("when the message is just at the emittable length", func() {
				BeforeEach(func() {
					message = strings.Repeat("7", log_streamer.MAX_MESSAGE_SIZE)
					Expect([]byte(message)).To(HaveLen(log_streamer.MAX_MESSAGE_SIZE), "Ensure that the byte representation of our message is under the limit")

					fmt.Fprintf(streamer.Stdout(), message+"\n")
				})

				It("should not break the message up and send a single messages", func() {
					Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))
					ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
					Expect(ms).To(Equal(message))
				})
			})

			Context("when the message exceeds the emittable length", func() {
				BeforeEach(func() {
					message = strings.Repeat("7", log_streamer.MAX_MESSAGE_SIZE)
					message += strings.Repeat("8", log_streamer.MAX_MESSAGE_SIZE)
					message += strings.Repeat("9", log_streamer.MAX_MESSAGE_SIZE)
					message += "hello\n"
					fmt.Fprintf(streamer.Stdout(), message)
				})

				It("should break the message up and send multiple messages", func() {
					Expect(fakeClient.SendAppLogCallCount()).To(Equal(4))

					ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
					Expect(ms).To(Equal(strings.Repeat("7", log_streamer.MAX_MESSAGE_SIZE)))
					ms, _, _ = fakeClient.SendAppLogArgsForCall(1)
					Expect(ms).To(Equal(strings.Repeat("8", log_streamer.MAX_MESSAGE_SIZE)))
					ms, _, _ = fakeClient.SendAppLogArgsForCall(2)
					Expect(ms).To(Equal(strings.Repeat("9", log_streamer.MAX_MESSAGE_SIZE)))
					ms, _, _ = fakeClient.SendAppLogArgsForCall(3)
					Expect(ms).To(Equal("hello"))
				})
			})

			Context("when having to deal with byte boundaries and long utf characters", func() {
				BeforeEach(func() {
					message = strings.Repeat("a", log_streamer.MAX_MESSAGE_SIZE-3)
					message += "\U0001F428\n"
				})

				It("should break the message up and send multiple messages without sending error runes", func() {
					fmt.Fprintf(streamer.Stdout(), message)
					Expect(fakeClient.SendAppLogCallCount()).To(Equal(2))

					ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
					Expect(ms).To(Equal(strings.Repeat("a", log_streamer.MAX_MESSAGE_SIZE-3)))
					ms, _, _ = fakeClient.SendAppLogArgsForCall(1)
					Expect(ms).To(Equal("\U0001F428"))
				})

				Context("with an invalid utf8 character in the message", func() {
					var utfChar string

					BeforeEach(func() {
						message = strings.Repeat("9", log_streamer.MAX_MESSAGE_SIZE-4)
						utfChar = "\U0001F428"
					})

					It("emits both messages correctly", func() {
						fmt.Fprintf(streamer.Stdout(), message+utfChar[0:2])
						fmt.Fprintf(streamer.Stdout(), utfChar+"\n")

						Expect(fakeClient.SendAppLogCallCount()).To(Equal(2))

						ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
						Expect(ms).To(Equal(message + utfChar[0:2]))

						ms, _, _ = fakeClient.SendAppLogArgsForCall(1)
						Expect(ms).To(Equal(utfChar))
					})
				})

				Context("when the entire message is invalid utf8 characters", func() {
					var utfChar string

					BeforeEach(func() {
						utfChar = "\U0001F428"
						message = strings.Repeat(utfChar[0:2], log_streamer.MAX_MESSAGE_SIZE/2)
						Expect(len(message)).To(Equal(log_streamer.MAX_MESSAGE_SIZE))
					})

					It("drops the last 3 bytes", func() {
						fmt.Fprintf(streamer.Stdout(), message)

						Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))

						ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
						Expect(ms).To(Equal(message[0 : len(message)-3]))
					})
				})
			})

			Context("while concatenating, if the message exceeds the emittable length", func() {
				BeforeEach(func() {
					message = strings.Repeat("7", log_streamer.MAX_MESSAGE_SIZE-2)
					fmt.Fprintf(streamer.Stdout(), message)
					fmt.Fprintf(streamer.Stdout(), "778888\n")
				})

				It("should break the message up and send multiple messages", func() {
					Expect(fakeClient.SendAppLogCallCount()).To(Equal(2))

					ms, _, _ := fakeClient.SendAppLogArgsForCall(0)
					Expect(ms).To(Equal(strings.Repeat("7", log_streamer.MAX_MESSAGE_SIZE)))
					ms, _, _ = fakeClient.SendAppLogArgsForCall(1)
					Expect(ms).To(Equal("8888"))
				})
			})
		})
	})

	Context("when told to emit stderr", func() {
		It("should handle short messages", func() {
			fmt.Fprintf(streamer.Stderr(), "this is a log\nand this is another\nand this one isn't done yet...")
			Expect(fakeClient.SendAppErrorLogCallCount()).To(Equal(2))

			msg, sn, _ := fakeClient.SendAppErrorLogArgsForCall(0)
			Expect(msg).To(Equal("this is a log"))
			Expect(sn).To(Equal(sourceName))

			msg, sn, _ = fakeClient.SendAppErrorLogArgsForCall(1)
			Expect(msg).To(Equal("and this is another"))
			Expect(sn).To(Equal(sourceName))
		})

		It("should handle long messages", func() {
			fmt.Fprintf(streamer.Stderr(), strings.Repeat("e", log_streamer.MAX_MESSAGE_SIZE+1)+"\n")
			Expect(fakeClient.SendAppErrorLogCallCount()).To(Equal(2))

			msg, _, _ := fakeClient.SendAppErrorLogArgsForCall(0)
			Expect(msg).To(Equal(strings.Repeat("e", log_streamer.MAX_MESSAGE_SIZE)))

			msg, _, _ = fakeClient.SendAppErrorLogArgsForCall(1)
			Expect(msg).To(Equal("e"))
		})
	})

	Context("when told to flush", func() {
		It("should send whatever log is left in its buffer", func() {
			fmt.Fprintf(streamer.Stdout(), "this is a stdout")
			fmt.Fprintf(streamer.Stderr(), "this is a stderr")

			Expect(fakeClient.SendAppLogCallCount()).To(Equal(0))
			Expect(fakeClient.SendAppErrorLogCallCount()).To(Equal(0))

			streamer.Flush()

			Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))
			Expect(fakeClient.SendAppErrorLogCallCount()).To(Equal(1))
		})
	})

	Context("when there is no app guid", func() {
		It("does nothing when told to emit or flush", func() {
			streamer = log_streamer.New(ctx, "", sourceName, index, tags, fakeClient)

			streamer.Stdout().Write([]byte("hi"))
			streamer.Stderr().Write([]byte("hi"))
			streamer.Flush()

			Expect(fakeClient.SendAppLogCallCount()).To(Equal(0))
		})
	})

	Context("when there is no log source", func() {
		It("defaults to LOG", func() {
			streamer = log_streamer.New(ctx, guid, "", -1, tags, fakeClient)

			streamer.Stdout().Write([]byte("hi"))
			streamer.Flush()

			Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))
			_, sn, _ := fakeClient.SendAppLogArgsForCall(0)
			Expect(sn).To(Equal(log_streamer.DefaultLogSource))
		})
	})

	Context("when there is no source index", func() {
		It("defaults to 0", func() {
			streamer = log_streamer.New(ctx, guid, sourceName, -1, tags, fakeClient)

			streamer.Stdout().Write([]byte("hi"))
			streamer.Flush()

			Expect(fakeClient.SendAppLogCallCount()).To(Equal(1))
			_, _, tags := fakeClient.SendAppLogArgsForCall(0)
			Expect(tags["instance_id"]).To(Equal("-1"))
		})
	})

	Context("with multiple goroutines emitting simultaneously", func() {
		var waitGroup *sync.WaitGroup

		BeforeEach(func() {
			waitGroup = new(sync.WaitGroup)

			for i := 0; i < 2; i++ {
				waitGroup.Add(1)
				go func() {
					defer waitGroup.Done()
					fmt.Fprintln(streamer.Stdout(), "this is a log")
				}()
			}
		})

		AfterEach(func(done Done) {
			defer close(done)
			waitGroup.Wait()
		})

		It("does not trigger data races", func() {
			Eventually(fakeClient.SendAppLogCallCount).Should(Equal(2))
		})
	})

	Context("when the log streamer context is cancelled", func() {
		BeforeEach(func() {
			cancelFunc()
		})

		It("writes to stdout and stderr should fail", func() {
			_, stdOutErr := fmt.Fprintln(streamer.Stdout(), "this is a log")
			Expect(stdOutErr).To(HaveOccurred())
			_, stdErrErr := fmt.Fprintln(streamer.Stderr(), "this is another log")
			Expect(stdErrErr).To(HaveOccurred())
		})
	})
})
