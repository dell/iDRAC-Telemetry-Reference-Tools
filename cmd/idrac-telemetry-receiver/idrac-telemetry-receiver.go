package main

import (
    "os"
    "os/exec"
)

func main() {
    // List of binaries to run
    cmds := []*exec.Cmd{
        exec.Command("/dbdiscauth"),
        exec.Command("/configui"),
        exec.Command("/redfishread"),
    }

    // Start all binaries
    for _, c := range cmds {
        c.Stdout = os.Stdout
        c.Stderr = os.Stderr
        if err := c.Start(); err != nil {
            panic(err)
        }
    }

    // Wait for all binaries to finish
    for _, c := range cmds {
        if err := c.Wait(); err != nil {
            // Optional: log error but continue waiting for others
            os.Stderr.WriteString(err.Error() + "\n")
        }
    }
}
