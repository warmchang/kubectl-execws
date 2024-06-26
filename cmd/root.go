package cmd

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/moby/term"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

var releaseVersion string

var cliopts Options

var rootCmd = &cobra.Command{
	Use:                   "kubectl-execws <pod name> [options] -- <cmd>",
	DisableFlagsInUseLine: true,
	Short:                 "kubectl exec over WebSockets",
	Long:                  `A replacement for "kubectl exec" that works over WebSocket connections.`,
	Args:                  cobra.MinimumNArgs(1),
	Version:               releaseVersion,
	SilenceUsage:          true,
	SilenceErrors:         true,
	/*CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd:   false,
		HiddenDefaultCmd:    true,
		DisableNoDescFlag:   true,
		DisableDescriptions: false,
	},*/
	RunE: func(cmd *cobra.Command, args []string) error {
		var object, pod string
		var command []string

		if strings.Contains(args[0], "/") {
			parts := strings.Split(args[0], "/")
			object = parts[0]
			pod = parts[1]
			command = args[1:]
		} else {
			object = "pod"
			pod = args[0]
			command = args[1:]
		}

		if object != "pod" {
			return errors.New("Non pod object not yet supported")
		}

		if len(command) == 0 {
			if cliopts.TTY {
				command = []string{"sh", "-c", "exec $(command -v bash || command -v ash || command -v sh)"}
			} else {
				return errors.New("Please specify a command")
			}
		}

		cliopts.Pod = pod
		cliopts.Object = object
		cliopts.Command = command

		s, err := NewCliSession(&cliopts)
		if err != nil {
			return err
		}

		if s.opts.noSanityCheck && s.opts.directExec {
			if s.opts.directExecNodeIp == "" {
				return errors.New("When using direct-exec you must either allow preflight request or provide node IP via --node-direct-exec-ip")
			}
			if s.opts.Container == "" {
				return errors.New("When using direct-exec you must either allow preflight request or provide target container name via -c")
			}
		}

		// propagate logging flags
		flag.Set("v", fmt.Sprint(cliopts.Loglevel))
		flag.Set("stderrthreshold", fmt.Sprint(cliopts.Loglevel))

		s.sanityCheck()

		var req *http.Request
		if s.opts.directExec {
			req, err = s.prepKubeletExec()
			if err != nil {
				return err
			}

		} else {
			req, err = s.prepExec()
			if err != nil {
				return err
			}
		}
		return s.doExec(req)

	},
	ValidArgsFunction: MainValidArgs,
}

// add our own explicit completion helper
var completionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	DisableFlagsInUseLine: true,
	Short:                 "Generate completion script",
	Long:                  fmt.Sprintf(`Generate the autocompletion script for %[1]s for the specified shell.`, rootCmd.Root().Name()),
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Hidden:                true,
	Run: func(cmd *cobra.Command, args []string) {
		_, stdOut, _ := term.StdStreams()
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletionV2(stdOut, true)
		case "zsh":
			cmd.Root().GenZshCompletion(stdOut)
		case "fish":
			cmd.Root().GenFishCompletion(stdOut, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(stdOut)
		}
	},
}

/*var versionCmd = &cobra.Command{
	Use:                   "version",
	Short:                 "Print program version",
	DisableFlagsInUseLine: true,
	Hidden:                true,
	Args:                  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(releaseVersion)
	},
}*/

func Execute() {
	klog.InitFlags(nil)

	err := rootCmd.Execute()
	if err != nil {
		klog.Exit(err)
	}
	os.Exit(0)
}

// shortcut to the hidden subcomand used for completion
func Complete() {
	rootCmd.SetArgs(append([]string{cobra.ShellCompRequestCmd}, os.Args[1:]...))
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cliopts.Kconfig, "kubeconfig", "", "kubeconfig file (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&cliopts.Namespace, "namespace", "n", "", "Set namespace")
	rootCmd.PersistentFlags().IntVarP(&cliopts.Loglevel, "loglevel", "v", 2, "Set loglevel")
	rootCmd.PersistentFlags().BoolVarP(&cliopts.noTLSVerify, "skip-tls-verify", "k", false, "Don't perform TLS certificate verifiation")
	rootCmd.PersistentFlags().StringVar(&cliopts.Impersonate, "as", "", "Impersonate another user")
	rootCmd.PersistentFlags().StringVar(&cliopts.Context, "context", "", "Use specific kubeconfig ctx")

	rootCmd.Flags().BoolVarP(&cliopts.TTY, "tty", "t", false, "Stdin is a TTY")
	rootCmd.Flags().BoolVarP(&cliopts.Stdin, "stdin", "i", false, "Pass stdin to container")
	rootCmd.Flags().StringVarP(&cliopts.Container, "container", "c", "", "Container name")
	rootCmd.Flags().BoolVar(&cliopts.noSanityCheck, "no-sanity-check", false, "Don't make preflight request to ensure pod exists")
	rootCmd.Flags().BoolVar(&cliopts.directExec, "node-direct-exec", false, "Partially bypass the API server, by using the kubelet API")
	rootCmd.Flags().StringVar(&cliopts.directExecNodeIp, "node-direct-exec-ip", "", "Node IP to use with direct-exec feature")

	rootCmd.AddCommand(completionCmd)
	//rootCmd.AddCommand(versionCmd)
	rootCmd.RegisterFlagCompletionFunc("namespace", NamespaceValidArgs)
	rootCmd.RegisterFlagCompletionFunc("container", ContainerValidArgs)
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}
