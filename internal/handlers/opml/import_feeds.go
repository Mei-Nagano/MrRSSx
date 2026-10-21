package opml

import (
	"context"
	"log"
	"strings"

	"MrRSS/internal/handlers/core"
	"MrRSS/internal/models"
)

// ImportFeeds stores parsed subscriptions as ordinary local feeds. A URL that
// is already subscribed is updated rather than duplicated, and articles for the
// imported feeds are fetched in the background so they show up without waiting
// for the next scheduled refresh.
//
// A failing entry is logged and skipped: one broken feed must not abort an
// import of hundreds. The returned IDs are the feeds that were created or
// updated.
func ImportFeeds(ctx context.Context, h *core.Handler, feeds []models.Feed) []int64 {
	if h == nil || len(feeds) == 0 {
		return nil
	}

	imported := make([]int64, 0, len(feeds))
	for _, f := range feeds {
		var (
			feedID int64
			err    error
		)

		if f.Type == "HTML+XPath" || f.Type == "XML+XPath" {
			feedID, err = h.Fetcher.AddXPathSubscription(
				f.URL, f.Category, f.Title, f.Type,
				f.XPathItem, f.XPathItemTitle, f.XPathItemContent, f.XPathItemUri,
				f.XPathItemAuthor, f.XPathItemTimestamp, f.XPathItemTimeFormat,
				f.XPathItemThumbnail, f.XPathItemCategories, f.XPathItemUid,
			)
		} else {
			feedID, err = h.Fetcher.ImportSubscription(f.Title, f.URL, f.Category)
		}

		if err != nil {
			log.Printf("Error importing feed %s: %v", f.Title, err)
			continue
		}

		if len(f.Tags) > 0 {
			if err := saveFeedTags(h, feedID, f.Tags); err != nil {
				log.Printf("Error saving tags for feed %s: %v", f.Title, err)
			}
		}

		imported = append(imported, feedID)
	}

	if len(imported) > 0 {
		// The browser response must not decide whether the first fetch happens, but
		// the imported feeds still belong to this request's lifetime boundaries.
		fetchCtx := context.WithoutCancel(ctx)
		go h.Fetcher.FetchFeedsByIDs(fetchCtx, imported)
	}

	return imported
}

// saveFeedTags creates or finds tags by name and associates them with the feed
func saveFeedTags(h *core.Handler, feedID int64, tags []models.Tag) error {
	if len(tags) == 0 {
		return nil
	}

	var tagIDs []int64
	for _, tag := range tags {
		// Check if tag already exists by name
		existingTags, err := h.DB.GetTags()
		if err != nil {
			log.Printf("Error fetching tags: %v", err)
			continue
		}

		var foundTagID int64
		for _, existingTag := range existingTags {
			if strings.EqualFold(existingTag.Name, tag.Name) {
				foundTagID = existingTag.ID
				break
			}
		}

		// If tag doesn't exist, create it
		if foundTagID == 0 {
			newTag := &models.Tag{
				Name:  tag.Name,
				Color: tag.Color,
			}
			id, err := h.DB.AddTag(newTag)
			if err != nil {
				log.Printf("Error creating tag %s: %v", tag.Name, err)
				continue
			}
			foundTagID = id
		}

		tagIDs = append(tagIDs, foundTagID)
	}

	// Associate tags with feed
	if len(tagIDs) > 0 {
		return h.DB.SetFeedTags(feedID, tagIDs)
	}

	return nil
}
