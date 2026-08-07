// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { app, setStore } from '../lib/state.svelte';
import type { FlowStore } from '../lib/store';
import TopBar from './TopBar.svelte';
import NewTaskDialog from './NewTaskDialog.svelte';

function mockStore() {
  return {
    projects: vi.fn(async () => []),
    nodes: vi.fn(async () => []),
    dependencies: vi.fn(async () => []),
    activity: vi.fn(async () => []),
    setStatus: vi.fn(async () => {}),
    setVerdict: vi.fn(async () => {}),
    addComment: vi.fn(async () => {}),
    createNode: vi.fn(async () => 'node-new'),
    updateNode: vi.fn(async () => {}),
    deleteNode: vi.fn(async () => {}),
    addDependency: vi.fn(async () => {}),
    removeDependency: vi.fn(async () => {}),
    undo: vi.fn(async () => {})
  } as unknown as FlowStore;
}

describe('New task flow', () => {
  beforeEach(() => {
    app.taskDialog = false;
    app.taskTitle = '';
    app.projectId = 'prj-travel';
  });

  it('opens the dialog from the TopBar button', async () => {
    setStore(mockStore());
    const { getByRole } = render(TopBar);
    await fireEvent.click(getByRole('button', { name: /new task/i }));
    expect(app.taskDialog).toBe(true);
  });

  it('creates a TASK via store.createNode when submitted', async () => {
    const store = mockStore();
    setStore(store);
    app.taskDialog = true;
    const { getByPlaceholderText, getByRole } = render(NewTaskDialog);

    await userEvent.type(getByPlaceholderText('Task title'), 'Write docs');
    await fireEvent.click(getByRole('button', { name: /create/i }));

    expect(store.createNode).toHaveBeenCalledTimes(1);
    expect(store.createNode).toHaveBeenCalledWith(
      expect.objectContaining({
        projectId: 'prj-travel',
        parentId: null,
        kind: 'TASK',
        title: 'Write docs'
      })
    );
  });
});
