package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentzhao/tricount-cli/internal/tricount"
	"github.com/spf13/cobra"
)

const maxUpload = 20 << 20

var attachmentCmd = &cobra.Command{
	Use:     "attachment",
	Aliases: []string{"attachments", "receipt", "receipts"},
	Short:   "Receipts and gallery images",
	Long: `Attachments are files stored on a group.

A transaction receipt is two steps:
  1. tricount attachment upload   returns a numeric id
  2. tricount attachment add      links that id to a transaction
You can also pass the id to tricount expense add --attachment while creating
the expense.

Gallery images are separate. They use tricount attachment gallery and are
identified by a UUID, not the numeric receipt id.

Subcommands:
  upload          Upload a receipt and print its id
  add             Link a receipt id to a transaction
  remove          Unlink a receipt id from a transaction
  gallery list    List gallery images
  gallery upload  Upload a gallery image
  gallery delete  Delete a gallery image

Next:
  tricount attachment upload --help
  tricount attachment gallery --help`,
	Example: `  tricount attachment upload --help
  tricount attachment gallery list --help`,
	RunE: helpOnly,
}

var attachmentUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a receipt and print its id",
	Args:  cobra.NoArgs,
	Long: `Upload a file and return the numeric id used on transactions.

The file is not attached to an expense until you pass the id to
tricount attachment add or tricount expense add --attachment.
Images and PDF files are accepted. The size limit is 20 MiB.

Next:
  tricount attachment add --help
  tricount expense add --help`,
	Example: `  tricount attachment upload --token tABC123xyz --file receipt.jpg`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		data, contentType, err := readUpload(cmd)
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		id, err := s.client.UploadAttachment(cmdCtx(cmd), tc.ID, contentType, data)
		if err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Uploaded %s as receipt id %d on %s.", flagString(cmd, "file"), id, tc.Title),
			Data: map[string]any{
				"id":           id,
				"content_type": contentType,
				"group":        viewSummary(tc),
			},
		})
	},
}

var attachmentAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Link a receipt to a transaction",
	Args:  cobra.NoArgs,
	Long: `Add a receipt id to an existing transaction.

--attachment is the numeric id from tricount attachment upload.
--transaction is the expense, income, or reimbursement id.
The current receipts on that transaction are kept.

Next:
  tricount expense get --help`,
	Example: `  tricount attachment add --token tABC123xyz --transaction 123 --attachment 456`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return changeAttachment(cmd, true)
	},
}

var attachmentRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Unlink a receipt from a transaction",
	Args:  cobra.NoArgs,
	Long: `Remove a receipt id from a transaction.

Other receipts on that transaction stay. Pass --yes. The CLI does not prompt.

Next:
  tricount expense get --help`,
	Example: `  tricount attachment remove --token tABC123xyz --transaction 123 --attachment 456 --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return changeAttachment(cmd, false)
	},
}

var galleryCmd = &cobra.Command{
	Use:   "gallery",
	Short: "Images stored on the group, separate from receipts",
	Long: `The gallery is a list of images on the group.

Gallery items are identified by UUID. Receipt ids from attachment upload
are a different kind of file and do not appear here.

Subcommands:
  list     Show gallery images and their URLs
  upload   Add an image and print its UUID
  delete   Remove an image by UUID

Next:
  tricount attachment gallery list --help
  tricount attachment gallery upload --help`,
	Example: `  tricount attachment gallery list --token tABC123xyz`,
	RunE:    helpOnly,
}

var galleryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List gallery images",
	Args:  cobra.NoArgs,
	Long: `List gallery images for a group.

original_url is the full-size URL when the API provided one. uuid is the
id to pass to gallery delete.

Next:
  tricount attachment gallery upload --help
  tricount attachment gallery delete --help`,
	Example: `  tricount attachment gallery list --token tABC123xyz`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := resolveRead(cmd)
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		items, err := s.client.ListGallery(cmdCtx(cmd), tc.ID)
		if err != nil {
			return err
		}
		views := make([]map[string]any, 0, len(items))
		for _, item := range items {
			views = append(views, map[string]any{
				"id":              item.ID,
				"uuid":            item.UUID,
				"attachment_uuid": item.AttachmentUUID,
				"content_type":    item.ContentType,
				"member_uuid":     item.MemberUUID,
				"original_url":    item.OriginalURL(),
			})
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("%s has %d gallery images.", tc.Title, len(views)),
			Data: map[string]any{
				"group":  viewSummary(tc),
				"images": views,
				"count":  len(views),
			},
		})
	},
}

var galleryUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a gallery image",
	Args:  cobra.NoArgs,
	Long: `Upload an image to the group gallery and print its UUID.

This does not attach the file to a transaction. Use tricount attachment upload
for a receipt. The size limit is 20 MiB.

Next:
  tricount attachment gallery list --help`,
	Example: `  tricount attachment gallery upload --token tABC123xyz --file photo.jpg`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		data, contentType, err := readUpload(cmd)
		if err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		id, err := s.client.UploadGallery(cmdCtx(cmd), tc.ID, contentType, data)
		if err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Uploaded %s to the gallery of %s as %s.", flagString(cmd, "file"), tc.Title, id),
			Data: map[string]any{
				"uuid":         id,
				"content_type": contentType,
				"group":        viewSummary(tc),
			},
		})
	},
}

var galleryDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a gallery image",
	Args:  cobra.NoArgs,
	Long: `Delete one gallery image by UUID.

The UUID comes from tricount attachment gallery list. Pass --yes.

Next:
  tricount attachment gallery list --help`,
	Example: `  tricount attachment gallery delete --token tABC123xyz --uuid 00000000-0000-4000-8000-000000000000 --yes`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tc, err := prepareExpenseGroup(cmd)
		if err != nil {
			return err
		}
		id := flagString(cmd, "uuid")
		if id == "" {
			return fmt.Errorf("pass --uuid from: tricount attachment gallery list --token %s", shellArg(tc.Token))
		}
		if err := confirm(fmt.Sprintf("delete gallery image %s from %s", id, tc.Title)); err != nil {
			return err
		}
		s, err := loadSession(cmdCtx(cmd))
		if err != nil {
			return err
		}
		if err := s.client.DeleteGallery(cmdCtx(cmd), tc.ID, id); err != nil {
			return err
		}
		return writeResult(cmd, Result{
			Summary: fmt.Sprintf("Deleted gallery image %s from %s.", id, tc.Title),
			Data:    map[string]any{"uuid": id, "group": viewSummary(tc)},
		})
	},
}

func changeAttachment(cmd *cobra.Command, add bool) error {
	tc, err := prepareExpenseGroup(cmd)
	if err != nil {
		return err
	}
	tx, err := findTransaction(tc, flagString(cmd, "transaction"))
	if err != nil {
		return err
	}
	attachmentID, err := cmd.Flags().GetInt("attachment")
	if err != nil {
		return err
	}
	if attachmentID <= 0 {
		return fmt.Errorf("pass --attachment with the numeric id from: tricount attachment upload --token %s --file <path>", shellArg(tc.Token))
	}
	ids := append([]int{}, tx.AttachmentIDs...)
	if add {
		for _, existing := range ids {
			if existing == attachmentID {
				return writeResult(cmd, Result{
					Summary: fmt.Sprintf("Transaction %d already has receipt %d.", tx.ID, attachmentID),
					Data:    map[string]any{"transaction": viewTransaction(tc, tx)},
				})
			}
		}
		ids = append(ids, attachmentID)
	} else {
		if err := confirm(fmt.Sprintf("remove receipt %d from transaction %d", attachmentID, tx.ID)); err != nil {
			return err
		}
		next := make([]int, 0, len(ids))
		found := false
		for _, existing := range ids {
			if existing == attachmentID {
				found = true
				continue
			}
			next = append(next, existing)
		}
		if !found {
			return fmt.Errorf("transaction %d does not have receipt %d. Current ids: %v. List the transaction with: tricount expense get --token %s --transaction %d", tx.ID, attachmentID, tx.AttachmentIDs, shellArg(tc.Token), tx.ID)
		}
		ids = next
	}
	payload := tricount.PreservePayload(tx, tc.Currency, ids)
	s, err := loadSession(cmdCtx(cmd))
	if err != nil {
		return err
	}
	if err := s.client.UpdateEntry(cmdCtx(cmd), tc.ID, tx.ID, payload); err != nil {
		return err
	}
	verb := "Added"
	if !add {
		verb = "Removed"
	}
	return writeUpdated(cmd, tc, tx.ID, fmt.Sprintf("%s receipt %d on transaction %d in %s.", verb, attachmentID, tx.ID, tc.Title))
}

func readUpload(cmd *cobra.Command) ([]byte, string, error) {
	path := flagString(cmd, "file")
	if path == "" {
		return nil, "", fmt.Errorf("pass --file with a path to an image or PDF")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	if info.Size() > maxUpload {
		return nil, "", fmt.Errorf("%s is %d bytes. The upload limit is %d bytes", path, info.Size(), maxUpload)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	contentType := flagString(cmd, "content-type")
	if contentType == "" {
		contentType = contentTypeFor(path)
	}
	return data, contentType, nil
}

func contentTypeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func bindFile(cmd *cobra.Command) {
	cmd.Flags().String("file", "", flagFileHelp)
	cmd.Flags().String("content-type", "", flagContentTypeHelp)
}

func init() {
	bindTarget(attachmentUploadCmd)
	bindFile(attachmentUploadCmd)
	bindTarget(attachmentAddCmd)
	attachmentAddCmd.Flags().String("transaction", "", flagTxHelp)
	attachmentAddCmd.Flags().Int("attachment", 0, "Receipt id from tricount attachment upload.")
	bindTarget(attachmentRemoveCmd)
	attachmentRemoveCmd.Flags().String("transaction", "", flagTxHelp)
	attachmentRemoveCmd.Flags().Int("attachment", 0, "Receipt id to unlink.")
	bindTarget(galleryListCmd)
	bindTarget(galleryUploadCmd)
	bindFile(galleryUploadCmd)
	bindTarget(galleryDeleteCmd)
	galleryDeleteCmd.Flags().String("uuid", "", "Gallery image UUID from tricount attachment gallery list.")
	galleryCmd.AddCommand(galleryListCmd, galleryUploadCmd, galleryDeleteCmd)
	attachmentCmd.AddCommand(attachmentUploadCmd, attachmentAddCmd, attachmentRemoveCmd, galleryCmd)
	rootCmd.AddCommand(attachmentCmd)
}
