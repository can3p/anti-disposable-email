// Copyright 2020-22 PJ Engineering and Business Solutions Pty. Ltd. All rights reserved.

package update

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
)

const header = `
// Copyright 2020-22 PJ Engineering and Business Solutions Pty. Ltd. All rights reserved.

package disposable

// DisposableList is the list of domains that are considered to be
// from disposable email service providers. See: https://github.com/martenson/disposable-email-domains.
//
// NOTE: To update the list, refer to the 'update' sub-package.
var DisposableList = map[string]struct{}{
`

const footer = `
}
`

func getPaddedFmt(maxLen int) string {
	// two spaces to keep gofmt happy
	return "\t%-" + strconv.Itoa(maxLen+2) + "q: {},\n"
}

func Update(ctx context.Context, target string) error {

	fs := memfs.New()

	opts := &git.CloneOptions{
		URL:   "https://github.com/disposable-email-domains/disposable-email-domains",
		Depth: 0,
	}

	_, err := git.CloneContext(ctx, memory.NewStorage(), fs, opts)
	if err != nil {
		return err
	}

	file, err := fs.Open("disposable_email_blocklist.conf")
	if err != nil {
		return err
	}

	newList := []string{}
	maxLength := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		domain := scanner.Text()

		if len(domain) > maxLength {
			maxLength = len(domain)
		}

		newList = append(newList, domain)
	}

	err = file.Close()
	if err != nil {
		return err
	}

	err = scanner.Err()
	if err != nil {
		return err
	}

	sort.SliceStable(newList, func(i, j int) bool {
		return newList[i] < newList[j]
	})

	tf, err := os.Create(target)

	if err != nil {
		return err
	}

	_, _ = tf.WriteString(header)

	format := getPaddedFmt(maxLength)

	for _, domain := range newList {
		_, _ = tf.WriteString(fmt.Sprintf(format, domain))
	}

	_, _ = tf.WriteString(footer)

	return nil
}
