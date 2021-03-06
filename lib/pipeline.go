package lib

import (
	"crypto/sha256"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

/*
The idea here is to have a pipeline, where:
1. Element enter
2. They are downloaded into local storange
3. They are ingested into CVMFS
4. Local resource are cleaned up
5. Element exits

What happen in case of failure?  We can return a structure that implement the
interface of the next pipeline, on this structure we can simply shortcut all
the other call or do the "right thing", re-try? Set a re-try in S3? Whatever!
*/

type PipelineInput interface {
	MakeS3RemoteFile() IS3RemoteFile
}

type IS3RemoteFile interface {
	DownloadFile() IS3LocalFile
}

type IS3LocalFile interface {
	Ingest() IS3IngestedFile
}

type IS3IngestedFile interface {
	Cleanup() PipelineOutput
}

type PipelineOutput struct{}

func NewPipeline() (chan<- PipelineInput, <-chan PipelineOutput) {
	// Each channel has a buffer of $buffer, and we have $workers running
	// at the same time, it means that in the worst case there are $buffer
	// + $workers job on the fly.
	buffer := 10
	workers := 10
	chanInput := make(chan PipelineInput, buffer)
	chanOutput := make(chan PipelineOutput, buffer)

	go func() {
		downloadChan := make(chan IS3RemoteFile, buffer)
		ingestChan := make(chan IS3LocalFile, buffer)
		cleanupChan := make(chan IS3IngestedFile, buffer)

		for w := 1; w <= workers; w++ {
			go func() {
				for pipelineInput := range chanInput {
					remoteFileToDownload :=
						pipelineInput.MakeS3RemoteFile()
					downloadChan <- remoteFileToDownload
				}
			}()

			go func() {
				for s3RemoteFile := range downloadChan {
					localFileToIngest := s3RemoteFile.DownloadFile()
					ingestChan <- localFileToIngest
				}
			}()

			go func() {
				for s3LocalFile := range ingestChan {
					ingestedFileToCleanup := s3LocalFile.Ingest()
					cleanupChan <- ingestedFileToCleanup
				}
			}()

			go func() {
				for s3IngestedFile := range cleanupChan {
					cleanedupFileToReturn := s3IngestedFile.Cleanup()
					chanOutput <- cleanedupFileToReturn
				}
			}()
		}
	}()

	return chanInput, chanOutput
}

type GenericError struct{}

func (e GenericError) DownloadFile() IS3LocalFile {
	return GenericError{}
}

func (e GenericError) Ingest() IS3IngestedFile {
	return GenericError{}
}

func (e GenericError) Cleanup() PipelineOutput {
	return PipelineOutput{}
}

type S3Object struct {
	Key     string
	Hash    string
	Session *session.Session
}

func NewS3Object(s3obj s3.Object, session *session.Session) S3Object {
	toHash := []byte(fmt.Sprintf("%s%d", *s3obj.Key, s3obj.LastModified.Unix()))
	hash := fmt.Sprintf("%s", sha256.Sum256(toHash))
	return S3Object{Key: *s3obj.Key, Hash: hash, Session: session}
}

func (s3obj S3Object) MakeS3RemoteFile() IS3RemoteFile {
	return s3obj
}

func (s3obj S3Object) DownloadFile() IS3LocalFile {
	return s3obj
}
