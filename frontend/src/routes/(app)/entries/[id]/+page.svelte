<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { 
    fetchEntry, 
    fetchTags, 
    updateEntry, 
    deleteEntry,
    addTagToEntry,
    removeTagFromEntry,
    createTag,
    fetchInteraction,
    upsertInteraction,
    deleteInteraction,
    fetchArchiveRevisions,
    refreshArchive,
    retryInManualMode,
    deleteArchiveRevision,
    loading, 
    error,
    type Entry,
    type Tag,
    type Interaction,
    type ArchiveRevision
  } from '$lib/api/client';
  import { fetchThumbnailCandidates, selectThumbnailCandidate, type ThumbnailCandidate } from '$lib/api/thumbnail';

  function getWhatsAppLink(phone: string): string {
    // Remove all non-numeric characters
    const clean = phone.replace(/\D/g, '');
    // Assume Brazil (+55) if no country code
    const withCountryCode = clean.length <= 11 ? '55' + clean : clean;
    return `https://wa.me/${withCountryCode}`;
  }

  let entry: Entry | null = null;
  let allTags: Tag[] = [];
  let statusPollTimer: ReturnType<typeof setInterval> | null = null;
  let interaction: Interaction | null = null;
  let isEditing = false;
  let isDeleting = false;
  let isEditingInteraction = false;
  let revisions: ArchiveRevision[] = [];
  let thumbnailCandidates: ThumbnailCandidate[] = [];
  let loadingCandidates = false;
  let selectingCandidateId: string | null = null;
  let thumbnailPickerOpen = false;
  let revisionHistoryOpen = false;
  let entryTags: Tag[] = [];
  let archiveRefreshKey = 0; // increment to force iframe refresh
  
  // Edit form
  let editForm = {
    title: '',
    description: '',
    phone_number: '',
    location: ''
  };

  // Interaction form
  let interactionForm = {
    tried: false,
    score: 1,
    comments: ''
  };

  // New tag form
  let newTagName = '';
  let newTagColor = '#3B82F6';

  $: entryId = $page.params.id ?? '';

  onMount(async () => {
    await loadData();
    startStatusPolling();
  });

  onDestroy(() => {
    stopStatusPolling();
  });

  async function loadData() {
    loading.set(true);
    try {
      entry = await fetchEntry(entryId);
      allTags = await fetchTags();
      interaction = await fetchInteraction(entryId);
      revisions = await fetchArchiveRevisions(entryId);
      if (entry) {
        editForm = {
          title: entry.title || '',
          description: entry.description || '',
          phone_number: entry.phone_number || '',
          location: entry.location || ''
        };
        await loadThumbnailCandidates(entry.id);
        if (entry.archive_status === 'pending') {
          startStatusPolling();
        } else {
          stopStatusPolling();
        }
      }
      if (interaction) {
        interactionForm = {
          tried: interaction.tried,
          score: interaction.score || 0,
          comments: interaction.comments || ''
        };
      }
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  async function handleSave() {
    if (!entry) return;
    loading.set(true);
    try {
      entry = await updateEntry(entry.id, editForm);
      isEditing = false;
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  async function handleDelete() {
    if (!entry || !confirm('Are you sure you want to delete this entry?')) return;
    isDeleting = true;
    try {
      await deleteEntry(entry.id);
      goto('/entries');
    } catch (e: any) {
      error.set(e.message);
      isDeleting = false;
    }
  }

  async function handleAddTag(tagId: string) {
    if (!entry) return;
    try {
      await addTagToEntry(entry.id, tagId);
      await loadData();
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function handleRemoveTag(tagId: string) {
    if (!entry) return;
    try {
      await removeTagFromEntry(entry.id, tagId);
      await loadData();
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function deleteRevision(revisionId: string) {
    if (!entry) return;
    if (!confirm('Are you sure you want to delete this revision? This cannot be undone.')) return;
    try {
      await deleteArchiveRevision(entry.id, revisionId);
      revisions = await fetchArchiveRevisions(entry.id);
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function handleSaveInteraction() {
    if (!entry) return;
    try {
      interaction = await upsertInteraction(entry.id, interactionForm);
      isEditingInteraction = false;
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function handleDeleteInteraction() {
    if (!entry || !interaction) return;
    if (!confirm('Are you sure you want to delete this interaction?')) return;
    try {
      await deleteInteraction(entry.id);
      interaction = null;
      interactionForm = { tried: false, score: 1, comments: '' };
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function handleThumbnailUpload(event: Event) {
    if (!entry) return;
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    const formData = new FormData();
    formData.append('file', file);

    try {
      loading.set(true);
      const response = await fetch(`/api/v1/files/thumbnails/${entry.id}`, {
        method: 'POST',
        body: formData,
        credentials: 'include'
      });

      if (!response.ok) {
        throw new Error('Failed to upload thumbnail');
      }

      // Reload entry to get updated timestamp for cache-busting
      entry = await fetchEntry(entry.id);
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  async function triggerRearchive() {
    if (!entry) return;
    try {
      await refreshArchive(entry.id);
      // Reset to show loading while new archive processes
      archiveRefreshKey++;
      await loadData();
      startStatusPolling();
    } catch (e: any) {
      error.set(e.message);
    }
  }

  async function triggerRetryManual() {
    if (!entry) return;
    const domain = new URL(entry.url).hostname;
    const makeDefault = confirm(`Use manual mode as default for ${domain}?\n\nThis will automatically use manual mode for future archives of this domain.`);
    try {
      await retryInManualMode(entry.id, makeDefault);
      archiveRefreshKey++;
      await loadData();
      startStatusPolling();
    } catch (e: any) {
      error.set(e.message);
    }
  }

  function startStatusPolling() {
    stopStatusPolling();
    console.log('[Polling] Starting status poll, entry.id:', entry?.id);
    statusPollTimer = setInterval(async () => {
      if (!entry?.id) return;
      try {
        console.log('[Polling] Fetching latest entry...');
        const latest = await fetchEntry(entry.id);
        console.log('[Polling] Got latest:', latest.archive_status, 'rev:', latest.archive_current_revision_id?.slice(0,8));
        entry = latest;
        if (latest.archive_status !== 'pending') {
          console.log('[Polling] Archive complete, stopping and refreshing...');
          stopStatusPolling();
          revisions = await fetchArchiveRevisions(entry.id);
          // Force iframe refresh with new revision
          archiveRefreshKey++;
          console.log('[Polling] Incremented archiveRefreshKey to', archiveRefreshKey);
          // Also refresh thumbnail candidates since new assets may be available
          await loadThumbnailCandidates(entry.id);
        }
      } catch (e) {
        console.error('[Polling] Error:', e);
      }
    }, 5000);
  }

  function stopStatusPolling() {
    if (statusPollTimer) {
      clearInterval(statusPollTimer);
      statusPollTimer = null;
    }
  }

  async function loadThumbnailCandidates(id: string) {
    loadingCandidates = true;
    try {
      thumbnailCandidates = await fetchThumbnailCandidates(id);
    } catch (e: any) {
      // non-blocking for detail page
      thumbnailCandidates = [];
    } finally {
      loadingCandidates = false;
    }
  }

  async function handleSelectCandidate(candidateId: string) {
    if (!entry) return;
    selectingCandidateId = candidateId;
    try {
      await selectThumbnailCandidate(entry.id, candidateId);
      entry = await fetchEntry(entry.id);
      await loadThumbnailCandidates(entry.id);
    } catch (e: any) {
      error.set(e.message);
    } finally {
      selectingCandidateId = null;
    }
  }

  async function handleCreateTag() {
    if (!newTagName.trim()) return;
    try {
      const tag = await createTag(newTagName, newTagColor);
      if (entry) {
        await addTagToEntry(entry.id, tag.id);
      }
      newTagName = '';
      await loadData();
    } catch (e: any) {
      error.set(e.message);
    }
  }

  function getStatusColor(status: string): string {
    switch (status) {
      case 'pending': return 'bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200';
      case 'processing': return 'bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200';
      case 'complete': return 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200';
      case 'failed': return 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200';
      default: return 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-200';
    }
  }

  $: entryTags = entry?.tags || [];
  $: availableTags = allTags.filter((t: Tag) => !entryTags.some((et: Tag) => et.id === t.id));

  // Compute archive base URL for thumbnail candidates
  $: archiveBaseUrl = entry?.id 
    ? (entry.archive_current_revision_id 
        ? `/api/v1/files/archive/${entry.id}/${entry.archive_current_revision_id}/`
        : `/api/v1/files/archive/${entry.id}/`)
    : '';
    
  function getCandidateImageUrl(path: string): string {
    if (!path) return '';
    return path.startsWith('/') ? path : archiveBaseUrl + path;
  }
</script>
<div class="px-4 sm:px-0">
  <div class="max-w-4xl mx-auto">
    <!-- Back Link -->
    <div class="mb-6">
      <a href="/entries" class="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 flex items-center gap-1">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
        Back to Entries
      </a>
    </div>

    {#if $loading && !entry}
      <div class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
      </div>
    {:else if $error && !entry}
      <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-400">
        {$error}
      </div>
    {:else if entry}
      <div class="bg-white dark:bg-slate-800 shadow rounded-lg overflow-hidden">
        <!-- Header -->
        <div class="px-4 py-5 sm:px-6 flex items-start justify-between">
          <div class="flex-1">
            {#if isEditing}
              <input
                type="text"
                bind:value={editForm.title}
                placeholder="Entry Title"
                class="block w-full text-xl font-bold border-b-2 border-blue-300 dark:border-blue-600 focus:border-blue-500 outline-none pb-1 bg-transparent text-slate-900 dark:text-white"
              />
            {:else}
              <h1 class="text-xl font-bold text-slate-900 dark:text-white">{entry.title || 'Untitled Entry'}</h1>
            {/if}
            <div class="mt-1 flex items-center gap-2">
              <a href={entry.url} target="_blank" class="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 truncate">{entry.url}</a>
              <a href={entry.url} target="_blank" class="text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 flex-shrink-0" title="Open original page">
                ↗
              </a>
            </div>
          </div>
          <span class="ml-4 inline-flex items-center px-3 py-1 rounded-full text-sm font-medium {getStatusColor(entry.archive_status)}">
            {entry.archive_status}
          </span>
        </div>

        <!-- Thumbnail -->
        <div class="aspect-video bg-slate-100 dark:bg-slate-700 flex items-center justify-center relative group">
          {#if entry.thumbnail_path}
            <img src="{entry.thumbnail_path}?t={entry.updated_at}" alt={entry.title} class="max-h-full max-w-full object-contain" />
          {:else}
            <svg class="h-24 w-24 text-slate-300 dark:text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          {/if}
          
          <!-- Upload overlay -->
          <div class="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
            <label class="cursor-pointer bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 px-4 py-2 rounded-md hover:bg-slate-50 dark:hover:bg-slate-700 text-sm font-medium">
              {entry.thumbnail_path ? 'Change Thumbnail' : 'Upload Thumbnail'}
              <input 
                type="file" 
                accept="image/*"
                class="hidden"
                on:change={handleThumbnailUpload}
              />
            </label>
          </div>
        </div>

        <!-- Thumbnail Picker (Collapsible) -->
        {#if thumbnailCandidates.length > 0}
          <div class="px-4 pt-4 sm:px-6">
            <button 
              type="button"
              on:click={() => thumbnailPickerOpen = !thumbnailPickerOpen}
              class="flex items-center justify-between w-full text-left"
            >
              <h3 class="text-sm font-medium text-slate-600 dark:text-slate-400">
                Thumbnail candidates 
                <span class="text-slate-400 dark:text-slate-500 font-normal">({thumbnailCandidates.length})</span>
              </h3>
              <span class="text-slate-400 dark:text-slate-500 text-xs">
                {thumbnailPickerOpen ? '▲' : '▼'}
              </span>
            </button>

            {#if thumbnailPickerOpen}
              <div class="mt-2 flex gap-2 overflow-x-auto pb-2 scrollbar-thin">
                {#each thumbnailCandidates as candidate}
                  <button
                    on:click={() => handleSelectCandidate(candidate.id)}
                    disabled={selectingCandidateId === candidate.id}
                    class={`flex-shrink-0 w-24 border rounded-md overflow-hidden hover:border-blue-500 ${candidate.selected ? 'border-blue-600 ring-2 ring-blue-200 dark:ring-blue-800' : 'border-slate-200 dark:border-slate-600'}`}
                  >
                    <img src={getCandidateImageUrl(candidate.path)} alt="Thumbnail candidate" class="w-full h-16 object-cover" />
                    <div class="px-1 py-0.5 text-[10px] bg-white dark:bg-slate-700 text-slate-600 dark:text-slate-400 flex items-center justify-between">
                      <span class="truncate">{candidate.source_type}</span>
                      {#if candidate.selected}
                        <span class="text-blue-600 dark:text-blue-400 font-semibold">✓</span>
                      {/if}
                    </div>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {:else if !loadingCandidates}
          <div class="px-4 pt-4 sm:px-6">
            <p class="text-xs text-slate-400 dark:text-slate-500">No candidates available yet. They will appear after archive assets are processed.</p>
          </div>
        {/if}

        <!-- Content -->
        <div class="px-4 py-5 sm:p-6">
          <!-- Error Message -->
          {#if $error}
            <div class="mb-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-400 text-sm">
              {$error}
            </div>
          {/if}

          {#if isEditing}
            <!-- Edit Form -->
            <div class="space-y-4">
              <div class="mb-4">
                <label for="description" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Description</label>
                <textarea
                  id="description"
                  bind:value={editForm.description}
                  rows="3"
                  class="mt-1 block w-full border border-slate-300 dark:border-slate-600 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm p-3 bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
                  placeholder="Add a description..."
                ></textarea>
              </div>
              <div class="mb-4">
                <label for="phone" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Phone</label>
                <input
                  type="text"
                  id="phone"
                  bind:value={editForm.phone_number}
                  class="mt-1 block w-full border border-slate-300 dark:border-slate-600 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm p-2 bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
                  placeholder="Phone number..."
                />
              </div>
              <div class="mb-4">
                <label for="location" class="block text-sm font-medium text-slate-700 dark:text-slate-300">Location</label>
                <input
                  type="text"
                  id="location"
                  bind:value={editForm.location}
                  class="mt-1 block w-full border border-slate-300 dark:border-slate-600 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm p-2 bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
                  placeholder="Location..."
                />
              </div>
              <div class="flex gap-2">
                <button
                  on:click={handleSave}
                  disabled={$loading}
                  class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
                >
                  Save
                </button>
                <button
                  on:click={() => isEditing = false}
                  class="px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm font-medium text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
                >
                  Cancel
                </button>
              </div>
            </div>
          {:else}
            <!-- View Mode -->
            <div class="space-y-4">
              {#if entry.description}
                <div>
                  <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Description</h3>
                  <p class="mt-1 text-sm text-slate-900 dark:text-white">{entry.description}</p>
                </div>
              {/if}
              
              {#if entry.phone_number}
                <div class="flex items-center gap-3">
                  <div>
                    <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Phone</h3>
                    <p class="mt-1 text-sm text-slate-900 dark:text-white">{entry.phone_number}</p>
                  </div>
                  <a
                    href={getWhatsAppLink(entry.phone_number)}
                    target="_blank"
                    rel="noopener noreferrer"
                    class="mt-4 p-2 rounded-full bg-green-100 dark:bg-green-900 hover:bg-green-200 dark:hover:bg-green-800 transition-colors"
                    title="Chat on WhatsApp"
                  >
                    <svg class="h-5 w-5 text-green-600 dark:text-green-400" fill="currentColor" viewBox="0 0 24 24">
                      <path d="M17.472 14.382c-.297-.149-1.758-.867-2.03-.967-.273-.099-.471-.148-.67.15-.197.297-.767.966-.94 1.164-.173.199-.347.223-.644.075-.297-.15-1.255-.463-2.39-1.475-.883-.788-1.48-1.761-1.653-2.059-.173-.297-.018-.458.13-.606.134-.133.298-.347.446-.52.149-.174.198-.298.298-.497.099-.198.05-.371-.025-.52-.075-.149-.669-1.612-.916-2.207-.242-.579-.487-.5-.669-.51-.173-.008-.371-.01-.57-.01-.198 0-.52.074-.792.372-.272.297-1.04 1.016-1.04 2.479 0 1.462 1.065 2.875 1.213 3.074.149.198 2.096 3.2 5.077 4.487.709.306 1.262.489 1.694.625.712.227 1.36.195 1.871.118.571-.085 1.758-.719 2.006-1.413.248-.694.248-1.289.173-1.413-.074-.124-.272-.198-.57-.347m-5.421 7.403h-.004a9.87 9.87 0 01-5.031-1.378l-.361-.214-3.741.982.998-3.648-.235-.374a9.86 9.86 0 01-1.51-5.26c.001-5.45 4.436-9.884 9.888-9.884 2.64 0 5.122 1.03 6.988 2.898a9.825 9.825 0 012.893 6.994c-.003 5.45-4.437 9.884-9.885 9.884m8.413-18.297A11.815 11.815 0 0012.05 0C5.495 0 .16 5.335.157 11.892c0 2.096.547 4.142 1.588 5.945L.057 24l6.305-1.654a11.882 11.882 0 005.683 1.448h.005c6.554 0 11.89-5.335 11.893-11.893a11.821 11.821 0 00-3.48-8.413z"/>
                    </svg>
                  </a>
                </div>
              {/if}

              {#if entry.location}
                <div>
                  <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Location</h3>
                  <p class="mt-1 text-sm text-slate-900 dark:text-white">{entry.location}</p>
                </div>
              {/if}

              <div>
                <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Tags</h3>
                <div class="mt-2 flex flex-wrap gap-2">
                  {#each entryTags as tag}
                    <span 
                      class="inline-flex items-center gap-1 px-3 py-1 rounded-full text-sm font-medium"
                      style="background-color: {tag.color}20; color: {tag.color}"
                    >
                      {tag.name}
                      <button
                        on:click={() => handleRemoveTag(tag.id)}
                        class="hover:opacity-70"
                      >
                        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                      </button>
                    </span>
                  {/each}
                  
                  {#if availableTags.length > 0}
                    <div class="relative group">
                      <button class="inline-flex items-center px-3 py-1 border border-dashed border-slate-300 dark:border-slate-600 rounded-full text-sm font-medium text-slate-500 dark:text-slate-400 hover:border-slate-400 dark:hover:border-slate-500">
                        + Add Tag
                      </button>
                      <div class="absolute left-0 mt-1 w-48 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-600 rounded-md shadow-lg hidden group-hover:block z-10">
                        {#each availableTags as tag}
                          <button
                            on:click={() => handleAddTag(tag.id)}
                            class="block w-full text-left px-4 py-2 text-sm hover:bg-slate-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300"
                          >
                            <span class="inline-block w-3 h-3 rounded-full mr-2" style="background-color: {tag.color}"></span>
                            {tag.name}
                          </button>
                        {/each}
                      </div>
                    </div>
                  {/if}
                </div>
              </div>

              <!-- Interaction Section -->
              <div class="border-t border-slate-200 dark:border-slate-700 pt-4 mt-4">
                <div class="flex items-center justify-between mb-3">
                  <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Interaction</h3>
                  {#if !isEditingInteraction}
                    <button
                      on:click={() => isEditingInteraction = true}
                      class="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300"
                    >
                      {interaction ? 'Edit' : '+ Add'}
                    </button>
                  {/if}
                </div>

                {#if isEditingInteraction}
                  <div class="space-y-3 bg-slate-50 dark:bg-slate-900 p-3 rounded-lg">
                    <label class="flex items-center gap-2">
                      <input 
                        type="checkbox" 
                        bind:checked={interactionForm.tried}
                        class="rounded border-slate-300 dark:border-slate-600"
                      />
                      <span class="text-sm text-slate-700 dark:text-slate-300">Tried / Contacted</span>
                    </label>

                    <div>
                      <label class="block text-sm text-slate-600 dark:text-slate-400 mb-1">Score (1-5)</label>
                      <div class="flex gap-1">
                        {#each [1, 2, 3, 4, 5] as score}
                          <button
                            type="button"
                            on:click={() => interactionForm.score = score}
                            class="w-8 h-8 rounded flex items-center justify-center {interactionForm.score >= score ? 'text-yellow-500' : 'text-slate-300 dark:text-slate-600'}"
                          >
                            ★
                          </button>
                        {/each}
                      </div>
                    </div>

                    <div>
                      <label class="block text-sm text-slate-600 dark:text-slate-400 mb-1">Comments</label>
                      <textarea
                        bind:value={interactionForm.comments}
                        rows="2"
                        class="w-full border border-slate-300 dark:border-slate-600 rounded-md p-2 text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
                        placeholder="Notes about this ad..."
                      ></textarea>
                    </div>

                    <div class="flex gap-2">
                      <button
                        on:click={handleSaveInteraction}
                        class="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700"
                      >
                        Save
                      </button>
                      <button
                        on:click={() => isEditingInteraction = false}
                        class="px-3 py-1.5 border border-slate-300 dark:border-slate-600 text-sm rounded-md hover:bg-slate-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300"
                      >
                        Cancel
                      </button>
                      {#if interaction}
                        <button
                          on:click={handleDeleteInteraction}
                          class="px-3 py-1.5 text-red-600 dark:text-red-400 text-sm hover:text-red-700 dark:hover:text-red-300"
                        >
                          Delete
                        </button>
                      {/if}
                    </div>
                  </div>
                {:else if interaction}
                  <div class="bg-slate-50 dark:bg-slate-900 p-3 rounded-lg">
                    <div class="flex items-center gap-3 mb-2">
                      {#if interaction.tried}
                        <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200">
                          ✓ Tried
                        </span>
                      {/if}
                      {#if interaction.score}
                        <span class="text-yellow-500">
                          {#each Array(5) as _, i}
                            <span class={i < interaction.score ? '' : 'text-slate-300 dark:text-slate-600'}>★</span>
                          {/each}
                        </span>
                      {/if}
                    </div>
                    {#if interaction.comments}
                      <p class="text-sm text-slate-700 dark:text-slate-300">{interaction.comments}</p>
                    {/if}
                  </div>
                {:else}
                  <p class="text-sm text-slate-400 dark:text-slate-500">No interaction recorded yet.</p>
                {/if}
              </div>

              <div>
                <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Created</h3>
                <p class="mt-1 text-sm text-slate-900 dark:text-white">{new Date(entry.created_at).toLocaleString()}</p>
              </div>

              <div>
                <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Archive Quality</h3>
                <div class="mt-1 flex items-center gap-2">
                  <span class={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${entry.archive_fidelity === 'high' ? 'bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300' : entry.archive_fidelity === 'partial' ? 'bg-yellow-100 dark:bg-yellow-900 text-yellow-700 dark:text-yellow-300' : 'bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-300'}`}>
                    {(entry.archive_fidelity || 'low').toUpperCase()}
                  </span>
                  {#if entry.archive_current_revision_id}
                    <span class="text-xs text-slate-500 dark:text-slate-400">Rev: {entry.archive_current_revision_id.slice(0, 8)}...</span>
                  {/if}
                </div>
              </div>

              {#if revisions.length > 0}
                <div>
                  <button 
                    type="button"
                    on:click={() => revisionHistoryOpen = !revisionHistoryOpen}
                    class="flex items-center justify-between w-full text-left"
                  >
                    <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">
                      Revision History <span class="text-slate-400 dark:text-slate-500 font-normal">({revisions.length})</span>
                    </h3>
                    <span class="text-slate-400 dark:text-slate-500 text-xs">
                      {revisionHistoryOpen ? '▲' : '▼'}
                    </span>
                  </button>

                  {#if revisionHistoryOpen}
                    <div class="mt-2 space-y-1">
                      {#each revisions as rev}
                        <div class="text-xs text-slate-600 dark:text-slate-400 flex items-center justify-between border border-slate-200 dark:border-slate-600 rounded px-2 py-1">
                          <span># {rev.revision_no} • {rev.status}</span>
                          <div class="flex items-center gap-2">
                            <span>{new Date(rev.captured_at).toLocaleString()}</span>
                            <a 
                              href={`/api/v1/files/archive/${entry.id}/${rev.id}/index.html`}
                              target="_blank"
                              class="text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300"
                              title="Open this revision in new tab"
                            >
                              ↗
                            </a>
                            <button
                              on:click={() => deleteRevision(rev.id)}
                              class="text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300"
                              title="Delete this revision"
                            >
                              🗑
                            </button>
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}

              {#if entry.archive_status === 'complete' || entry.archive_status === 'success'}
                <div>
                  <details class="group">
                    <summary class="cursor-pointer list-none">
                      <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400 flex items-center gap-2">
                        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                        </svg>
                        Archived Content
                        <span class="text-xs text-slate-400 dark:text-slate-500 group-open:rotate-180 transition-transform">
                          ▼
                        </span>
                      </h3>
                    </summary>
                    <div class="mt-3 border border-slate-200 dark:border-slate-600 rounded-lg overflow-hidden">
                      <div class="bg-slate-50 dark:bg-slate-900 px-3 py-2 border-b border-slate-200 dark:border-slate-600 flex items-center justify-between">
                        <span class="text-xs text-slate-500 dark:text-slate-400">Status: {entry.archive_status}</span>
                        <div class="flex gap-2">
                          <a 
                            href={entry.archive_current_revision_id 
                              ? `/api/v1/files/archive/${entry.id}/${entry.archive_current_revision_id}/index.html`
                              : `/api/v1/files/archive/${entry.id}`}
                            target="_blank"
                            class="text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300"
                          >
                            Open in new tab
                          </a>
                          <button
                            on:click={triggerRearchive}
                            class="text-xs text-slate-600 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-300"
                          >
                            Re-archive
                          </button>
                        </div>
                      </div>
                      {#key archiveRefreshKey}
                        <iframe 
                          src={entry.archive_current_revision_id 
                            ? `/api/v1/files/archive/${entry.id}/${entry.archive_current_revision_id}/index.html`
                            : `/api/v1/files/archive/${entry.id}`}
                          class="w-full h-96 bg-white dark:bg-slate-700"
                          title="Archived content"
                          sandbox="allow-same-origin"
                        ></iframe>
                      {/key}
                    </div>
                  </details>
                </div>
              {:else if entry.archive_status === 'pending'}
                <div>
                  <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Archive Status</h3>
                  <p class="mt-1 text-sm text-yellow-600 dark:text-yellow-400 flex items-center gap-2">
                    <svg class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Archiving in progress...
                  </p>
                </div>
              {:else if entry.archive_status === 'failed' || entry.archive_status === 'blocked'}
                <div>
                  <h3 class="text-sm font-medium text-slate-500 dark:text-slate-400">Archive Status</h3>
                  <div class="mt-1 flex flex-col gap-2">
                    <span class="text-sm {entry.archive_status === 'blocked' ? 'text-orange-600 dark:text-orange-400' : 'text-red-600 dark:text-red-400'}">{entry.archive_status === 'blocked' ? 'Archive blocked (challenge detected)' : 'Archive failed'}</span>
                    <button
                      on:click={triggerRearchive}
                      class="text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300"
                    >
                      Retry
                    </button>
                    <button
                      on:click={triggerRetryManual}
                      class="text-xs text-orange-600 dark:text-orange-400 hover:text-orange-700 dark:hover:text-orange-300 font-medium"
                      title="Retry using manual/interactive browser mode"
                    >
                      Retry in Manual Mode
                    </button>
                  </div>
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Actions -->
        <div class="px-4 py-4 sm:px-6 bg-slate-50 dark:bg-slate-900 flex items-center justify-between">
          <button
            on:click={handleDelete}
            disabled={isDeleting}
            class="px-4 py-2 text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 text-sm font-medium disabled:opacity-50"
          >
            {#if isDeleting}
              Deleting...
            {:else}
              Delete Entry
            {/if}
          </button>
          
          {#if !isEditing}
            <button
              on:click={() => isEditing = true}
              class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700"
            >
              Edit
            </button>
          {/if}
        </div>
      </div>
    {/if}
  </div>
</div>