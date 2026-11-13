package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// startVocabReview snapshots the due words into a queue. The queue holds IDs,
// not entry copies: grading one card must not hide edits made elsewhere, and a
// word deleted while the session is open has to disappear rather than keep its
// stale text on screen.
func (a *app) startVocabReview() {
	due := GetVocabularyStore().DueEntries(time.Now())
	ids := make([]string, 0, len(due))
	for _, e := range due {
		ids = append(ids, e.ID)
	}
	a.vocabReviewIDs = ids
	a.vocabReviewIdx = 0
	a.vocabReviewRevealed = false
	a.vocabReviewDone = 0
	a.vocabReviewing = true
	a.settingsRefresh()
}

func (a *app) endVocabReview() {
	a.vocabReviewing = false
	a.vocabReviewIDs = nil
	a.vocabReviewIdx = 0
	a.vocabReviewRevealed = false
	a.settingsRefresh()
}

// openVocabReview is the tray entry point: it opens the settings window on the
// notebook page and starts a session in one action.
func (a *app) openVocabReview() {
	a.page = "vocabulary"
	a.showSettings()
	a.startVocabReview()
}

func (a *app) gradeVocabReview(grade reviewGrade) {
	if a.vocabReviewIdx >= len(a.vocabReviewIDs) {
		return
	}
	id := a.vocabReviewIDs[a.vocabReviewIdx]
	if _, ok := GetVocabularyStore().ApplyReview(id, grade, time.Now()); ok {
		a.vocabReviewDone++
	}
	a.vocabReviewIdx++
	a.vocabReviewRevealed = false
	a.settingsRefresh()
}

// pageVocabReview is the flashcard surface. It replaces the list while a
// session is active, so the grade buttons are never mixed up with the list's
// delete and expand controls.
func (a *app) pageVocabReview(c *ui.Context, t *ui.Theme) {
	total := len(a.vocabReviewIDs)
	if a.vocabReviewIdx >= total {
		a.reviewSummary(c, t)
		return
	}

	store := GetVocabularyStore()
	// Entries can be deleted from the list while a session is queued. Skip
	// them instead of ending the whole session on the first stale ID.
	var entry VocabularyEntry
	found := false
	for a.vocabReviewIdx < total {
		if e, ok := store.Get(a.vocabReviewIDs[a.vocabReviewIdx]); ok {
			entry, found = e, true
			break
		}
		a.vocabReviewIdx++
		a.vocabReviewRevealed = false
	}
	if !found {
		a.reviewSummary(c, t)
		return
	}

	PageTitle(c, t, "生词复习", "先回忆，再显示答案。三个按钮决定这个词下次出现的时间。")
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Progress(c, float64(a.vocabReviewIdx)/float64(total)).Grow(1)
		ui.Textf(c, "剩余 %d / %d", total-a.vocabReviewIdx, total).FontSize(12).TextColor(t.TextMuted)
	})

	ui.Box(c).FillWidth().Padding(28, 26).Radius(12).Background(t.Surface).
		Border(1, t.Border).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.05)).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, entry.Term).FontSize(30).FontWeight(700).TextColor(t.Text).Wrap()
			if entry.Phonetic != "" {
				ui.Text(c, entry.Phonetic).FontSize(14).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if entry.PartOfSpeech != "" {
					ui.Box(c).Padding(1, 6).Radius(4).Background(t.SurfaceHover).Children(func() {
						ui.Text(c, entry.PartOfSpeech).FontSize(11).FontWeight(600).TextColor(t.Accent)
					})
				}
				if entry.Difficulty != "" {
					d := entry.Difficulty
					bg, fg := cefrColors(t, d)
					ui.Box(c).Padding(1, 6).Radius(10).Background(bg).Children(func() {
						ui.Text(c, d).FontSize(10).FontWeight(700).TextColor(fg)
					})
				}
			})

			if !a.vocabReviewRevealed {
				ui.Text(c, "回想它的意思，然后显示答案。").FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, "显示答案").Height(32).Padding(4, 18).Radius(6).Clicked() {
					a.vocabReviewRevealed = true
					a.settingsRefresh()
				}
				return
			}

			ui.Divider(c).FillWidth()
			if entry.Definition != "" {
				ui.Text(c, entry.Definition).FontSize(15).TextColor(t.Text).Wrap()
			} else {
				ui.Text(c, "这个词没有保存释义。").FontSize(13).TextColor(t.TextMuted)
			}
			if entry.Context != "" {
				ui.Text(c, entry.Context).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if entry.ContextTranslation != "" {
				ui.Text(c, entry.ContextTranslation).FontSize(12).TextColor(t.TextMuted).Wrap()
			}
			if entry.MemoryTips != "" {
				ui.Text(c, "💡 "+entry.MemoryTips).FontSize(12).TextColor(t.TextMuted).Wrap()
			}

			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "忘记").Height(30).Padding(3, 16).Radius(6).Clicked() {
					a.gradeVocabReview(reviewAgain)
				}
				if ui.Button(c, "模糊").Height(30).Padding(3, 16).Radius(6).Clicked() {
					a.gradeVocabReview(reviewHard)
				}
				if ui.Button(c, "记得").Height(30).Padding(3, 16).Radius(6).Background(t.Accent).TextColor(ui.Hex("#FFFFFF")).Clicked() {
					a.gradeVocabReview(reviewGood)
				}
			})
			ui.Text(c, "忘记：10 分钟后 · 模糊：间隔小幅延长 · 记得：间隔翻倍").FontSize(11).TextColor(t.TextMuted)
		})
	})

	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "结束本轮复习").Height(26).Padding(2, 10).FontSize(11).Clicked() {
			a.endVocabReview()
		}
	})
}

func (a *app) reviewSummary(c *ui.Context, t *ui.Theme) {
	PageTitle(c, t, "复习完成", fmt.Sprintf("本轮复习了 %d 个词。到期的词会重新回到这里。", a.vocabReviewDone))
	ui.Box(c).FillWidth().Padding(32, 24).Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			if a.vocabReviewDone == 0 && len(a.vocabReviewIDs) == 0 {
				ui.Text(c, "今天没有到期的生词。").FontSize(15).FontWeight(600).TextColor(t.Text)
				ui.Text(c, "新保存的词会立即进入复习队列；已经复习过的词会按间隔回来。").FontSize(12).TextColor(t.TextMuted).Wrap()
			} else {
				ui.Text(c, "🎉").FontSize(26)
				ui.Text(c, "今天的词复习完了").FontSize(16).FontWeight(600).TextColor(t.Text)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "再来一轮").Height(28).Padding(2, 12).Radius(5).Clicked() {
					a.startVocabReview()
				}
				if ui.Button(c, "返回生词本").Height(28).Padding(2, 12).Radius(5).Clicked() {
					a.endVocabReview()
				}
			})
		})
	})
}
