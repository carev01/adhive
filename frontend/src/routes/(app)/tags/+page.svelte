<script lang="ts">
  import { onMount } from 'svelte';
  import { fetchTags, createTag, updateTag, deleteTag, loading, error, type Tag } from '$lib/api/client';

  let tags: Tag[] = [];
  let showModal = false;
  let editingTag: Tag | null = null;
  let isDeleting = false;
  let deletingTagId = '';

  // Form state
  let formName = '';
  let formColor = '#3B82F6';

  // Predefined colors
  const colors = [
    '#3B82F6', // Blue
    '#10B981', // Green
    '#F59E0B', // Amber
    '#EF4444', // Red
    '#8B5CF6', // Purple
    '#EC4899', // Pink
    '#06B6D4', // Cyan
    '#84CC16', // Lime
    '#F97316', // Orange
    '#6366F1', // Indigo
  ];

  onMount(async () => {
    await loadTags();
  });

  async function loadTags() {
    loading.set(true);
    try {
      tags = await fetchTags();
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  function openCreateModal() {
    editingTag = null;
    formName = '';
    formColor = '#3B82F6';
    showModal = true;
  }

  function openEditModal(tag: Tag) {
    editingTag = tag;
    formName = tag.name;
    formColor = tag.color;
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    editingTag = null;
    formName = '';
    formColor = '#3B82F6';
  }

  async function handleSubmit() {
    if (!formName.trim()) return;
    
    loading.set(true);
    try {
      if (editingTag) {
        await updateTag(editingTag.id, formName, formColor);
      } else {
        await createTag(formName, formColor);
      }
      await loadTags();
      closeModal();
    } catch (e: any) {
      error.set(e.message);
    } finally {
      loading.set(false);
    }
  }

  async function handleDelete(tagId: string) {
    if (!confirm('Are you sure you want to delete this tag? It will be removed from all entries.')) return;
    
    isDeleting = true;
    deletingTagId = tagId;
    try {
      await deleteTag(tagId);
      await loadTags();
    } catch (e: any) {
      error.set(e.message);
    } finally {
      isDeleting = false;
      deletingTagId = '';
    }
  }
</script>

<div class="px-4 sm:px-0">
  <!-- Header -->
  <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
    <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Tags</h1>
    <button 
      on:click={openCreateModal}
      class="inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700"
    >
      Create Tag
    </button>
  </div>

  <!-- Error -->
  {#if $error}
    <div class="mb-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4 text-red-700 dark:text-red-400 text-sm">
      {$error}
    </div>
  {/if}

  <!-- Loading -->
  {#if $loading}
    <div class="flex justify-center py-12">
      <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
    </div>
  {:else if tags.length === 0}
    <div class="bg-white dark:bg-slate-800 shadow rounded-lg p-12 text-center">
      <svg class="mx-auto h-12 w-12 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
      </svg>
      <h3 class="mt-2 text-sm font-medium text-slate-900 dark:text-white">No tags</h3>
      <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">Create tags to organize your entries.</p>
      <div class="mt-6">
        <button 
          on:click={openCreateModal}
          class="inline-flex items-center px-4 py-2 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700"
        >
          Create Tag
        </button>
      </div>
    </div>
  {:else}
    <!-- Tags Grid -->
    <div class="bg-white dark:bg-slate-800 shadow rounded-lg overflow-hidden">
      <table class="min-w-full divide-y divide-slate-200 dark:divide-slate-700">
        <thead class="bg-slate-50 dark:bg-slate-900">
          <tr>
            <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400 uppercase tracking-wider">
              Tag
            </th>
            <th scope="col" class="px-6 py-3 text-left text-xs font-medium text-slate-500 dark:text-slate-400 uppercase tracking-wider">
              Color
            </th>
            <th scope="col" class="px-6 py-3 text-right text-xs font-medium text-slate-500 dark:text-slate-400 uppercase tracking-wider">
              Actions
            </th>
          </tr>
        </thead>
        <tbody class="bg-white dark:bg-slate-800 divide-y divide-slate-200 dark:divide-slate-700">
          {#each tags as tag}
            <tr>
              <td class="px-6 py-4 whitespace-nowrap">
                <span 
                  class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium"
                  style="background-color: {tag.color}20; color: {tag.color}"
                >
                  {tag.name}
                </span>
              </td>
              <td class="px-6 py-4 whitespace-nowrap">
                <div class="flex items-center gap-2">
                  <span 
                    class="inline-block w-6 h-6 rounded-full border border-slate-200 dark:border-slate-600"
                    style="background-color: {tag.color}"
                  ></span>
                  <span class="text-sm text-slate-500 dark:text-slate-400">{tag.color}</span>
                </div>
              </td>
              <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                <button
                  on:click={() => openEditModal(tag)}
                  class="text-blue-600 dark:text-blue-400 hover:text-blue-900 dark:hover:text-blue-300 mr-4"
                >
                  Edit
                </button>
                <button
                  on:click={() => handleDelete(tag.id)}
                  disabled={isDeleting && deletingTagId === tag.id}
                  class="text-red-600 dark:text-red-400 hover:text-red-900 dark:hover:text-red-300 disabled:opacity-50"
                >
                  {isDeleting && deletingTagId === tag.id ? 'Deleting...' : 'Delete'}
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Modal -->
{#if showModal}
  <!-- Background overlay - click to close -->
  <button
    class="fixed inset-0 z-40 bg-slate-500/75 dark:bg-slate-900/75 cursor-default"
    on:click={closeModal}
    aria-label="Close modal"
  ></button>

  <!-- Modal panel -->
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-white dark:bg-slate-800 rounded-lg shadow-xl max-w-md w-full p-6 relative">
      <h3 class="text-lg font-medium text-slate-900 dark:text-white mb-4">
        {editingTag ? 'Edit Tag' : 'Create Tag'}
      </h3>

      <form on:submit|preventDefault={handleSubmit} class="space-y-4">
        <!-- Name -->
        <div>
          <label for="tag-name" class="block text-sm font-medium text-slate-700 dark:text-slate-300">
            Name
          </label>
          <input
            type="text"
            id="tag-name"
            bind:value={formName}
            placeholder="Enter tag name"
            class="mt-1 block w-full border border-slate-300 dark:border-slate-600 rounded-md shadow-sm focus:ring-blue-500 focus:border-blue-500 sm:text-sm px-3 py-2 bg-white dark:bg-slate-700 text-slate-900 dark:text-white placeholder-slate-400 dark:placeholder-slate-500"
          />
        </div>

        <!-- Color -->
        <div>
          <label for="tag-color" class="block text-sm font-medium text-slate-700 dark:text-slate-300">
            Color
          </label>
          <div id="tag-color" class="mt-2 flex flex-wrap gap-2">
            {#each colors as c}
              <button
                type="button"
                on:click={() => formColor = c}
                class="inline-block w-8 h-8 rounded-full border-2 transition-transform hover:scale-110"
                class:border-slate-900={formColor === c}
                class:border-transparent={formColor !== c}
                style="background-color: {c}"
              ></button>
            {/each}
          </div>
          <div class="mt-2 flex items-center gap-2">
            <span
              class="inline-block w-6 h-6 rounded-full border border-slate-200 dark:border-slate-600"
              style="background-color: {formColor}"
            ></span>
            <span class="text-sm text-slate-500 dark:text-slate-400">{formColor}</span>
          </div>
        </div>

        <!-- Preview -->
        <div>
          <span class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">Preview</span>
          <span
            class="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium"
            style="background-color: {formColor}20; color: {formColor}"
          >
            {formName || 'Tag Name'}
          </span>
        </div>

        <!-- Actions -->
        <div class="flex justify-end gap-3 pt-2">
          <button
            type="button"
            on:click={closeModal}
            class="px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-md text-sm font-medium text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={$loading || !formName.trim()}
            class="px-4 py-2 border border-transparent rounded-md text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50"
          >
            {$loading ? 'Saving...' : (editingTag ? 'Save Changes' : 'Create')}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}