// Command validate checks the dataset: shape, references and provenance.
// Errors fail the build; "seed" rules are listed as warnings so contributors
// can pick one up.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Wander-Kit/borderkit"
)

func main() {
	var d *borderkit.Dataset
	var err error
	if len(os.Args) > 1 {
		d, err = borderkit.LoadFS(os.DirFS(os.Args[1]), ".")
	} else {
		d, err = borderkit.Load()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	msgs := d.Validate()
	errs, warns := 0, 0
	for _, m := range msgs {
		fmt.Println(m)
		if strings.HasPrefix(m, "E ") {
			errs++
		} else {
			warns++
		}
	}
	fmt.Printf("\n%d countries, %d groups, %d errors, %d rules still need a source\n", len(d.CountryCodes()), len(d.Groups), errs, warns)
	if errs > 0 {
		os.Exit(1)
	}
}
