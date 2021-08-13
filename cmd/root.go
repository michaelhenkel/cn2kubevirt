package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	file string
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(deleteCmd)
}

func initConfig() {
}

var rootCmd = &cobra.Command{
	Use:   "cn2kubevirt",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
