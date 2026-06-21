// Package s3 implements the object Store interface using Amazon S3.
//
// This is a stub — the full implementation lands in a downstream change.
// The AWS SDK dependency lives only in this subpackage.
package s3

import "fmt"

func init() {
	fmt.Println("s3 storage stub registered")
}
