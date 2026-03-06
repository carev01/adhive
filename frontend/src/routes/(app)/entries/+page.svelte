<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { afterNavigate } from '$app/navigation';
  import { fetchEntries, fetchTags, fetchRandomEntry, loading, error, type Entry, type Tag } from '$lib/api/client';

  let entries: Entry[] = [];
  let tags: Tag[] = [];
  let selectedTag = '';
  let searchQuery = '';
  let page = 1;
  let total = 0;
  let limit = 20;
  let searchTimeout: ReturnType<typeof setTimeout>;
  let excludeTried = false;
  let randomLoading = false;
  let listPollTimer: ReturnType<typeof setInterval> | null = null;

  // Reactive
  $: totalPages = Math.ceil(total / limit);

  onMount(async () => {
    await loadTags();
    await loadEntries();
    startListPolling();
  });

  onDestroy(() => {
    stopListPolling();
  });

  // Reload when navigating back to this page
  afterNavigate(({ from }) => {
    if (from?.url?.pathname?.startsWith('/entries/') && from.url.pathname !== '/entries/new') {
      loadEntries();
    }
  });

  async function loadTags() {
    try {
      tags = await fetchTags();
    } catch (e) {
      console.error('Failed to load tags:', e);
    }
  }

  async function loadEntries() {
    loading.set(true);
    error.set(null);
    try {
      const result = await fetchEntries({
        page,
        limit,
        tag: selectedTag || undefined,
        search: searchQuery || undefined,
        exclude_tried: excludeTried
      });
      entries = result.entries;
      total = result.total;
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  function handleSearch() {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
      page = 1;
      loadEntries();
    }, 300);
  }

  function handleTagFilter(tagId: string) {
    selectedTag = tagId;
    page = 1;
    loadEntries();
  }

  async function handleRandomPick() {
    randomLoading = true;
    try {
      const params = {
        exclude_tried: excludeTried,
        include_tags: selectedTag ? [selectedTag] : undefined
      };
      console.log('Pick Random params:', params);
      const entry = await fetchRandomEntry(params);
      console.log('Random entry picked:', entry);
      goto(`/entries/${entry.id}`);
    } catch (e: any) {
      console.error('Random pick error:', e);
      error.set(e.message);
    } finally {
      randomLoading = false;
    }
  }

  function prevPage() {
    if (page > 1) {
      page--;
      loadEntries();
    }
  }

  function nextPage() {
    if (page < totalPages) {
      page++;
      loadEntries();
    }
  }

  function getStatusColor(status: string): string {
    switch (status) {
      case 'pending': return 'bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200';
      case 'processing': return 'bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200';
      case 'complete':
      case 'success': return 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200';
      case 'failed': return 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-200';
      default: return 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-200';
    }
  }

  function startListPolling() {
    stopListPolling();
    listPollTimer = setInterval(async () => {
      if (entries.some((e) => e.archive_status === 'pending')) {
        await loadEntries();
      }
    }, 5000);
  }

  function stopListPolling() {
    if (listPollTimer) {
      clearInterval(listPollTimer);
      listPollTimer = null;
    }
  }
</script>

<div class="px-4 sm:px-0">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
    <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Entries</h1>
    <a 
      href="/entries/new" 
      class="inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700"
    >
      Add Entry
    </a>
  </div>

  <!-- Search & Filters -->
  <div class="bg-white dark:bg-slate-800 shadow rounded-lg mb-6 p-4">
    <div class="flex flex-col sm:flex-row gap-4">
      <!-- Search -->
      <div class="flex-1">
        <label for="search" class="sr-only">Search</label>
        <div class="relative">
          <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
            <svg class="h-5 w-5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </div>
          <input
            type="text"
            id="search"
            bind:value={searchQuery}
            on:input={handleSearch}
            placeholder="Search entries..."
            class="block w-full pl-10 pr-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md leading-5 bg-white dark:bg-slate-700 placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:placeholder-slate-400 dark:focus:placeholder-slate-500 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 sm:text-sm text-slate-900 dark:text-white"
          />
        </div>
      </div>
      
      <!-- Tag Filter -->
      <div class="sm:w-48">
        <select
          bind:value={selectedTag}
          on:change={() => handleTagFilter(selectedTag)}
          class="block w-full py-2 pl-3 pr-10 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
        >
          <option value="">All Tags</option>
          {#each tags as tag}
            <option value={tag.id}>{tag.name}</option>
          {/each}
        </select>
      </div>

      <!-- Random Pick -->
      <div class="flex items-center gap-2">
        <label class="flex items-center gap-2 text-sm cursor-pointer {excludeTried ? 'text-purple-400 font-medium' : 'text-slate-600 dark:text-slate-400'}">
          <input 
            type="checkbox" 
            bind:checked={excludeTried}
            on:change={() => loadEntries()}
            class="rounded border-slate-300 dark:border-slate-600 text-purple-600 focus:ring-purple-500"
          />
          Exclude tried
          {#if excludeTried}
            <span class="inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200">
              Active
            </span>
          {/if}
        </label>
        <button
          on:click={handleRandomPick}
          disabled={randomLoading}
          class="inline-flex items-center px-4 py-2 border border-purple-300 dark:border-purple-700 text-purple-700 dark:text-purple-300 bg-purple-50 dark:bg-purple-900/30 rounded-md hover:bg-purple-100 dark:hover:bg-purple-900/50 disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
        >
          {#if randomLoading}
            <svg class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            Picking...
          {:else}
            🎲 Pick Random
          {/if}
        </button>
      </div>
    </div>
  </div>

  <!-- Loading State -->
  {#if $loading}
    <div class="flex justify-center py-12">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
    </div>
  {:else if $error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-400">
      {$error}
    </div>
  {:else if entries.length === 0}
    <div class="bg-white dark:bg-slate-800 shadow rounded-lg p-12 text-center">
      <svg class="mx-auto h-12 w-12 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
      </svg>
      <h3 class="mt-2 text-sm font-medium text-slate-900 dark:text-white">No entries</h3>
      <p class="mt-1 text-sm text-slate-500">Get started by adding your first entry.</p>
      <div class="mt-6">
        <a href="/entries/new" class="inline-flex items-center px-4 py-2 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700">
          Add Entry
        </a>
      </div>
    </div>
  {:else}
    <!-- Entries Grid -->
    <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {#each entries as entry}
        <a 
          href="/entries/{entry.id}"
          class="bg-white dark:bg-slate-800 shadow rounded-lg p-4 hover:shadow-md transition-shadow"
        >
          <!-- Thumbnail -->
          <div class="aspect-video bg-slate-100 dark:bg-slate-700 rounded-md mb-3 flex items-center justify-center overflow-hidden">
            {#if entry.thumbnail_path}
              <img src="{entry.thumbnail_path}?t={entry.updated_at}" alt={entry.title} class="w-full h-full object-cover" />
            {:else}
              <svg class="h-12 w-12 text-slate-300 dark:text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
              </svg>
            {/if}
          </div>
          
          <!-- Content -->
          <h3 class="text-sm font-medium text-slate-900 dark:text-white truncate">{entry.title || entry.url}</h3>
          <p class="mt-1 text-xs text-slate-500 dark:text-slate-400 line-clamp-2">{entry.description || 'No description'}</p>
          
          <!-- Meta -->
          <div class="mt-3 flex items-center justify-between">
            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium {getStatusColor(entry.archive_status)}">
              {entry.archive_status}
            </span>
            {#if entry.tags && entry.tags.length > 0}
              <div class="flex gap-1">
                {#each entry.tags.slice(0, 2) as tag}
                  <span 
                    class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
                    style="background-color: {tag.color}20; color: {tag.color}"
                  >
                    {tag.name}
                  </span>
                {/each}
                {#if entry.tags.length > 2}
                  <span class="text-xs text-slate-400">+{entry.tags.length - 2}</span>
                {/if}
              </div>
            {/if}
          </div>
        </a>
      {/each}
    </div>

    <!-- Pagination -->
    {#if totalPages > 1}
      <div class="mt-6 flex items-center justify-between">
        <div class="text-sm text-slate-700 dark:text-slate-300">
          Showing {((page - 1) * limit) + 1} to {Math.min(page * limit, total)} of {total} entries
        </div>
        <div class="flex gap-2">
          <button
            on:click={prevPage}
            disabled={page === 1}
            class="px-3 py-1 border border-slate-300 dark:border-slate-600 rounded-md text-sm disabled:opacity-50 disabled:cursor-not-allowed hover:bg-slate-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300"
          >
            Previous
          </button>
          <button
            on:click={nextPage}
            disabled={page === totalPages}
            class="px-3 py-1 border border-slate-300 dark:border-slate-600 rounded-md text-sm disabled:opacity-50 disabled:cursor-not-allowed hover:bg-slate-50 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300"
          >
            Next
          </button>
        </div>
      </div>
    {/if}
  {/if}
</div>
