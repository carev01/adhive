<script lang="ts">
  import { goto } from '$app/navigation';
  import { createEntry, loading, error } from '$lib/api/client';

  let url = '';
  let submitting = false;
  let formError = '';

  async function handleSubmit() {
    if (!url) {
      formError = 'URL is required';
      return;
    }

    // Basic URL validation
    try {
      new URL(url);
    } catch {
      formError = 'Please enter a valid URL';
      return;
    }

    submitting = true;
    formError = '';

    try {
      const entry = await createEntry(url);
      goto(`/entries/${entry.id}`);
    } catch (e: any) {
      formError = e.message || 'Failed to create entry';
    } finally {
      submitting = false;
    }
  }
</script>

<div class="px-4 sm:px-0">
  <div class="max-w-2xl mx-auto">
    <!-- Header -->
    <div class="mb-6">
      <a href="/entries" class="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 flex items-center gap-1">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
        Back to Entries
      </a>
    </div>

    <div class="bg-white dark:bg-slate-800 shadow rounded-lg">
      <div class="px-4 py-5 sm:p-6">
        <h1 class="text-lg font-medium text-slate-900 dark:text-white mb-6">Add New Entry</h1>

        {#if formError}
          <div class="mb-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-400 text-sm">
            {formError}
          </div>
        {/if}

        <form on:submit|preventDefault={handleSubmit} class="space-y-6">
          <!-- URL Input -->
          <div>
            <label for="url" class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              URL <span class="text-red-500">*</span>
            </label>
            <input
              type="url"
              id="url"
              bind:value={url}
              placeholder="https://example.com/ad-page"
              class="block w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-md shadow-sm focus:outline-none focus:ring-blue-500 focus:border-blue-500 sm:text-sm bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
              disabled={submitting}
            />
            <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
              Enter the URL of the ad or listing page you want to archive
            </p>
          </div>

          <!-- Info Box -->
          <div class="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
            <div class="flex">
              <svg class="h-5 w-5 text-blue-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <div class="ml-3">
                <h3 class="text-sm font-medium text-blue-800 dark:text-blue-300">How it works</h3>
                <div class="mt-1 text-xs text-blue-700 dark:text-blue-400">
                  <p>When you submit a URL, our system will:</p>
                  <ul class="list-disc list-inside mt-1 space-y-0.5">
                    <li>Fetch and analyze the page content</li>
                    <li>Extract relevant metadata (title, description, images)</li>
                    <li>Create an archived copy for offline access</li>
                    <li>Archive status will update as processing completes</li>
                  </ul>
                </div>
              </div>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex justify-end gap-3 pt-4">
            <a
              href="/entries"
              class="px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm font-medium text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
            >
              Cancel
            </a>
            <button
              type="submit"
              disabled={submitting}
              class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {#if submitting}
                <svg class="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
                Processing...
              {:else}
                Add Entry
              {/if}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</div>