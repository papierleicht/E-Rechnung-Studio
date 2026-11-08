// Command release-sign creates the update signing key and signs or checks the
// packages of a release for the in-app updater (see internal/updatesign).
//
//	release-sign keygen -out FILE   writes a new private key to FILE (mode 0600)
//	                                and prints its public key
//	release-sign sign FILE...       writes FILE.sig with the key from the
//	                                environment variable UPDATE_SIGNING_KEY
//	release-sign verify FILE...     checks FILE.sig against the public key
//	                                built into the app
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"papierleicht.de/e-rechnung-studio/internal/updatesign"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		var err error
		switch args[0] {
		case "keygen":
			err = keygen(args[1:], stdout)
		case "sign":
			err = sign(args[1:], stdout)
		case "verify":
			err = verify(args[1:], stdout)
		default:
			usage(stderr)
			return 2
		}
		if err != nil {
			fmt.Fprintln(stderr, "release-sign:", err)
			return 1
		}
		return 0
	}
	usage(stderr)
	return 2
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: release-sign keygen -out FILE")
	fmt.Fprintln(w, "       release-sign sign FILE...    (key in UPDATE_SIGNING_KEY)")
	fmt.Fprintln(w, "       release-sign verify FILE...")
}

func keygen(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := flags.String("out", "", "file for the private key; must not exist")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" || flags.NArg() > 0 {
		return errors.New("keygen needs -out FILE and nothing else")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	// O_EXCL: never overwrite a key that may already sign releases.
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = io.WriteString(f, updatesign.EncodePrivateKey(priv)+"\n"); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "private key: %s\npublic key:  %s\n", *out, updatesign.EncodePublicKey(pub))
	return nil
}

// sign refuses a key that does not belong to the public key built into the
// app: the app would reject every package signed with it.
func sign(files []string, stdout io.Writer) error {
	if len(files) == 0 {
		return errors.New("sign needs at least one file")
	}
	key, err := updatesign.ParsePrivateKey(os.Getenv("UPDATE_SIGNING_KEY"))
	if err != nil {
		return fmt.Errorf("UPDATE_SIGNING_KEY: %w", err)
	}
	pub, err := updatesign.ParsePublicKey(updatesign.PublicKey)
	if err != nil {
		return err
	}
	if !pub.Equal(key.Public()) {
		return errors.New("UPDATE_SIGNING_KEY does not belong to the public key in internal/updatesign; the app would reject these packages")
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		sig, err := updatesign.Sign(key, bytes.NewReader(data))
		if err != nil {
			return err
		}
		if err = os.WriteFile(name+updatesign.Suffix, []byte(sig), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "signed", name)
	}
	return nil
}

func verify(files []string, stdout io.Writer) error {
	if len(files) == 0 {
		return errors.New("verify needs at least one file")
	}
	pub, err := updatesign.ParsePublicKey(updatesign.PublicKey)
	if err != nil {
		return err
	}
	for _, name := range files {
		sig, err := os.ReadFile(name + updatesign.Suffix)
		if err != nil {
			return err
		}
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		err = updatesign.Verify(pub, f, string(sig))
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintln(stdout, "verified", name)
	}
	return nil
}
