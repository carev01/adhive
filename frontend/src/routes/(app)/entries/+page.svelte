<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { afterNavigate } from '$app/navigation';
  import { fetchEntries, fetchTags, fetchRandomEntry, fetchSources, loading, error, type Entry, type Tag } from '$lib/api/client';

  // Core state
  let entries: Entry[] = [];
  let tags: Tag[] = [];
  let sources: string[] = [];
  
  // Filter state
  let selectedTag = '';
  let searchQuery = '';
  let excludeTried = false;
  let statusFilter = '';
  let dateFrom = '';
  let dateTo = '';
  let sourceFilter = '';
  
  // Interaction filters
  let hasInteraction = false;
  let minScore = 0;
  
  // Sort state
  let sortBy = 'created_at-desc';
  
  // UI state
  let page = 1;
  let total = 0;
  let limit = 20;
  let searchTimeout: ReturnType<typeof setTimeout>;
  let randomLoading = false;
  let listPollTimer: ReturnType<typeof setInterval> | null = null;
  let showFilters = false;

  // Reactive
  $: totalPages = Math.ceil(total / limit);
  $: hasActiveFilters = statusFilter || dateFrom || dateTo || sourceFilter || hasInteraction || minScore > 0;

  onMount(async () => {
    await Promise.all([loadTags(), loadSources()]);
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

  async function loadSources() {
    try {
      sources = await fetchSources();
    } catch (e) {
      console.error('Failed to load sources:', e);
    }
  }

  async function loadEntries() {
    // Parse sort value
    const [sortField, sortDir] = sortBy.split('-') as [string, string];
    
    loading.set(true);
    error.set(null);
    try {
      const result = await fetchEntries({
        page,
        limit,
        tag: selectedTag || undefined,
        search: searchQuery || undefined,
        exclude_tried: excludeTried,
        status: statusFilter || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        source: sourceFilter || undefined,
        sort_by: sortField as 'created_at' | 'updated_at' | 'title',
        sort_order: sortDir as 'asc' | 'desc',
        has_interaction: hasInteraction || undefined,
        min_score: minScore > 0 ? minScore : undefined,
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

  function handleFilterChange() {
    page = 1;
    loadEntries();
  }

  function handleSortChange() {
    // Parse sort value (format: "field-direction")
    const [field, direction] = sortBy.split('-') as [string, string];
    page = 1;
    loadEntriesWithSort(field as 'created_at' | 'updated_at' | 'title', direction as 'asc' | 'desc');
  }

  async function loadEntriesWithSort(field: 'created_at' | 'updated_at' | 'title', direction: 'asc' | 'desc') {
    loading.set(true);
    error.set(null);
    try {
      const result = await fetchEntries({
        page,
        limit,
        tag: selectedTag || undefined,
        search: searchQuery || undefined,
        exclude_tried: excludeTried,
        status: statusFilter || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        source: sourceFilter || undefined,
        sort_by: field,
        sort_order: direction,
        has_interaction: hasInteraction || undefined,
        min_score: minScore > 0 ? minScore : undefined,
      });
      entries = result.entries;
      total = result.total;
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  function clearFilters() {
    statusFilter = '';
    dateFrom = '';
    dateTo = '';
    sourceFilter = '';
    hasInteraction = false;
    minScore = 0;
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

  <!-- Search & Quick Filters -->
  <div class="bg-white dark:bg-slate-800 shadow rounded-lg mb-4 p-4">
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
            class="block w-full pl-10 pr-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md leading-5 bg-white dark:bg-slate-700 placeholder-slate-500 dark:placeholder-slate-400 focus:outline-none focus:placeholder-slate-400 focus:ring-1 focus:ring-blue-500 focus:border-blue-500 sm:text-sm text-slate-900 dark:text-white"
          />
        </div>
      </div>
      
      <!-- Tag Filter -->
      <div class="sm:w-40">
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

      <!-- Sort Dropdown -->
      <div class="sm:w-48">
        <select
          bind:value={sortBy}
          on:change={handleSortChange}
          class="block w-full py-2 pl-3 pr-10 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
        >
          <option value="created_at-desc">Date Added (Newest)</option>
          <option value="created_at-asc">Date Added (Oldest)</option>
          <option value="title-asc">Title (A-Z)</option>
          <option value="title-desc">Title (Z-A)</option>
          <option value="updated_at-desc">Last Updated (Newest)</option>
          <option value="updated_at-asc">Last Updated (Oldest)</option>
        </select>
      </div>

      <!-- Toggle Filters Button -->
      <button
        on:click={() => showFilters = !showFilters}
        class="inline-flex items-center px-4 py-2 border border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 bg-white dark:bg-slate-700 rounded-md hover:bg-slate-50 dark:hover:bg-slate-600 text-sm font-medium"
      >
        <svg class="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
        </svg>
        Filters
        {#if hasActiveFilters}
          <span class="ml-1.5 inline-flex items-center px-1.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
            Active
          </span>
        {/if}
      </button>

      <!-- Random Pick -->
      <div class="flex items-center gap-2">
        <label class="flex items-center gap-2 text-sm cursor-pointer {excludeTried ? 'text-purple-400 font-medium' : 'text-slate-600 dark:text-slate-400'}">
          <input 
            type="checkbox" 
            bind:checked={excludeTried}
            on:change={() => loadEntries()}
            class="rounded border-slate-300 dark:border-slate-600 text-purple-600 dark:text-purple-400 focus:ring-purple-500 dark:focus:ring-purple-400 bg-white dark:bg-slate-700"
          />
          Exclude tried
        </label>
        <button
          on:click={handleRandomPick}
          disabled={randomLoading}
          class="inline-flex items-center px-4 py-2 border border-purple-300 dark:border-purple-700 text-purple-700 dark:text-purple-400 bg-purple-50 dark:bg-purple-900/30 rounded-md hover:bg-purple-100 dark:hover:bg-purple-900/50 disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
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

    <!-- Expanded Filters Panel -->
    {#if showFilters}
      <div class="mt-4 pt-4 border-t border-slate-200">
        <div class="flex flex-col sm:flex-row gap-4">
          <!-- Status Filter -->
          <div class="sm:w-40">
            <label for="status-filter" class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">Archive Status</label>
            <select
              id="status-filter"
              bind:value={statusFilter}
              on:change={handleFilterChange}
              class="block w-full py-2 pl-3 pr-10 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
            >
              <option value="">All Statuses</option>
              <option value="pending">Pending</option>
              <option value="processing">Processing</option>
              <option value="success">Success</option>
              <option value="failed">Failed</option>
            </select>
          </div>

          <!-- Source Filter -->
          <div class="sm:w-48">
            <label for="source-filter" class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">Source</label>
            <select
              id="source-filter"
              bind:value={sourceFilter}
              on:change={handleFilterChange}
              class="block w-full py-2 pl-3 pr-10 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
            >
              <option value="">All Sources</option>
              {#each sources as source}
                <option value={source}>{source}</option>
              {/each}
            </select>
          </div>

          <!-- Date From -->
          <div class="sm:w-40">
            <label for="date-from" class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">Date From</label>
            <input
              type="date"
              id="date-from"
              bind:value={dateFrom}
              on:change={handleFilterChange}
              class="block w-full py-2 px-3 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
            />
          </div>

          <!-- Date To -->
          <div class="sm:w-40">
            <label for="date-to" class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">Date To</label>
            <input
              type="date"
              id="date-to"
              bind:value={dateTo}
              on:change={handleFilterChange}
              class="block w-full py-2 px-3 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
            />
          </div>

          <!-- Has Interaction Toggle -->
          <div class="flex items-center">
            <label class="flex items-center gap-2 text-sm cursor-pointer text-slate-700 dark:text-slate-300">
              <input 
                type="checkbox" 
                bind:checked={hasInteraction}
                on:change={handleFilterChange}
                class="rounded border-slate-300 dark:border-slate-600 text-blue-600 dark:text-blue-400 focus:ring-blue-500 dark:focus:ring-blue-400 bg-white dark:bg-slate-700"
              />
              Has Interaction
            </label>
          </div>

          <!-- Min Score Selector -->
          <div class="sm:w-40">
            <label for="min-score" class="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">Min Score</label>
            <select
              id="min-score"
              bind:value={minScore}
              on:change={handleFilterChange}
              class="block w-full py-2 pl-3 pr-10 border border-slate-300 dark:border-slate-600 rounded-md focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white"
            >
              <option value={0}>Any</option>
              <option value={1}>1+ ⭐</option>
              <option value={2}>2+ ⭐</option>
              <option value={3}>3+ ⭐</option>
              <option value={4}>4+ ⭐</option>
              <option value={5}>5 ⭐</option>
            </select>
          </div>

          <!-- Clear Filters Button -->
          {#if hasActiveFilters}
            <div class="flex items-end">
              <button
                on:click={clearFilters}
                class="inline-flex items-center px-3 py-2 border border-red-300 dark:border-red-700 text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/30 rounded-md hover:bg-red-100 dark:hover:bg-red-900/50 text-sm font-medium"
              >
                <svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
                Clear Filters
              </button>
            </div>
          {/if}
        </div>
      </div>
    {/if}
  </div>

  <!-- Active Filters Summary -->
  {#if hasActiveFilters}
    <div class="mb-4 flex flex-wrap gap-2">
      {#if statusFilter}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200">
          Status: {statusFilter}
          <button on:click={() => { statusFilter = ''; handleFilterChange(); }} class="ml-1.5 hover:text-blue-900 dark:hover:text-blue-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
      {#if sourceFilter}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-200">
          Source: {sourceFilter}
          <button on:click={() => { sourceFilter = ''; handleFilterChange(); }} class="ml-1.5 hover:text-green-900 dark:hover:text-green-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
      {#if dateFrom}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200">
          From: {dateFrom}
          <button on:click={() => { dateFrom = ''; handleFilterChange(); }} class="ml-1.5 hover:text-purple-900 dark:hover:text-purple-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
      {#if dateTo}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-purple-100 dark:bg-purple-900 text-purple-800 dark:text-purple-200">
          To: {dateTo}
          <button on:click={() => { dateTo = ''; handleFilterChange(); }} class="ml-1.5 hover:text-purple-900 dark:hover:text-purple-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
      {#if hasInteraction}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-indigo-100 dark:bg-indigo-900 text-indigo-800 dark:text-indigo-200">
          Has Interaction
          <button on:click={() => { hasInteraction = false; handleFilterChange(); }} class="ml-1.5 hover:text-indigo-900 dark:hover:text-indigo-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
      {#if minScore > 0}
        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-100 dark:bg-amber-900 text-amber-800 dark:text-amber-200">
          Min Score: {minScore}+ ⭐
          <button on:click={() => { minScore = 0; handleFilterChange(); }} class="ml-1.5 hover:text-amber-900 dark:hover:text-amber-100">
            <svg class="h-3 w-3" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
            </svg>
          </button>
        </span>
      {/if}
    </div>
  {/if}

  <!-- Loading State -->
  {#if $loading}
    <div class="flex justify-center py-12">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
    </div>
  {:else if $error}
    <div class="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700">
      {$error}
    </div>
  {:else if entries.length === 0}
    <div class="bg-white dark:bg-slate-800 shadow rounded-lg p-12 text-center">
      <svg class="mx-auto h-12 w-12 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
      </svg>
      <h3 class="mt-2 text-sm font-medium text-slate-900 dark:text-white">No entries</h3>
      <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">Get started by adding your first entry.</p>
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
              <svg class="h-12 w-12 text-slate-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
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
