# Pending Task: Generate NotebookLM Audio/Video Overviews

**Created:** 2026-03-24  
**Priority:** High  
**Status:** ⏸️ Blocked - API Limits

## Context

NotebookLM notebook created with 227 merged specifications:
- **Notebook ID:** `b6316edb-9c76-4ae4-bd88-d6ded28c324f`
- **URL:** https://notebooklm.google.com/notebook/b6316edb-9c76-4ae4-bd88-d6ded28c324f
- **Sources:** 5 batch files (merged-specs-001-050.md through 201-227.md)

## What Was Attempted

Attempted to generate via API:
```python
notebooklm_studio_create(artifact_type="audio", audio_format="deep_dive", audio_length="long")
notebooklm_studio_create(artifact_type="video", video_format="explainer", visual_style="auto_select")
```

**Result:** API returned errors - "Could not create audio/video"

**Likely Cause:** API rate limits or audio/video generation requires web UI interaction

## What Needs To Be Done

### When API Limits Clear (Try Again):

```bash
# Audio Overview - Deep Dive Podcast (~10-15 min)
notebooklm_studio_create(
    notebook_id="b6316edb-9c76-4ae4-bd88-d6ded28c324f",
    artifact_type="audio",
    audio_format="deep_dive",
    audio_length="long",
    confirm=True
)

# Video Overview - Visual Explainer (~3-5 min)
notebooklm_studio_create(
    notebook_id="b6316edb-9c76-4ae4-bd88-d6ded28c324f",
    artifact_type="video",
    video_format="explainer",
    visual_style="classic",
    confirm=True
)

# Download when complete
notebooklm_download_artifact(
    notebook_id="b6316edb-9c76-4ae4-bd88-d6ded28c324f",
    artifact_type="audio",
    output_path="./merged-specs/audio-overview.mp3"
)

notebooklm_download_artifact(
    notebook_id="b6316edb-9c76-4ae4-bd88-d6ded28c324f",
    artifact_type="video",
    output_path="./merged-specs/video-overview.mp4"
)
```

### Alternative: Generate via Web UI

If API continues to fail:

1. **Open:** https://notebooklm.google.com/notebook/b6316edb-9c76-4ae4-bd88-d6ded28c324f
2. **Click "Audio Overview"** button (top right corner)
3. **Wait ~2-5 minutes** for generation
4. **Download MP3** when ready
5. **Repeat for Video Overview**

## Purpose

These overviews help with:
- **Audio:** Passive learning during commute/walk - hear AI discuss the architecture
- **Video:** Visual walkthrough with animations - see system relationships
- **Both:** Deep understanding of what we're building before Ralph loop continues

## Related Artifacts (Already Generated)

| Artifact | Status | Location |
|----------|--------|----------|
| Study Guide | ✅ Complete | `merged-specs/study-guide.md` |
| Mind Map | ✅ Complete | `merged-specs/architecture-mindmap.json` |
| Flashcards | ✅ Complete | `merged-specs/flashcards.md` |
| Quiz | ✅ Complete | `merged-specs/quiz.md` |
| Slide Deck | ⏳ In Progress | - |
| Audio Overview | ⏸️ Blocked | - |
| Video Overview | ⏸️ Blocked | - |

## Trigger Conditions

Try again when:
- [ ] 24+ hours since last attempt (rate limit reset)
- [ ] OR error message changes from "Could not create" to something actionable
- [ ] OR NotebookLM API documentation updates with audio/video support

## Success Criteria

- [ ] Audio overview downloaded to `merged-specs/audio-overview.mp3`
- [ ] Video overview downloaded to `merged-specs/video-overview.mp4`
- [ ] Both files playable and contain relevant spec content
- [ ] Duration: Audio 10-15 min, Video 3-5 min

---

**Related Files:**
- `.memory-bank/notebooklm-setup.md` - Initial setup notes
- `merged-specs/` - All batch files and generated artifacts
- `TODO.md` - Main task queue (audio/video not yet added as tasks)

**Next Action:** Wait 24 hours, then retry API calls or use web UI
