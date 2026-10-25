import { get } from 'svelte/store';
import { openTrash, closeTrash } from '../../modules/trash/controller';
import { activeTab } from './mobile_shell_store';

let returnTab: 'files' | 'account' | null = null;

export function clearTrashReturnTab(): void { returnTab = null; }

export function openMobileTrash(origin: 'files' | 'account'): void {
    returnTab = origin;
    openTrash();
    activeTab.set('files');
}

export function closeMobileTrash(): void {
    const destination = returnTab;
    returnTab = null;
    closeTrash();
    if (destination === 'account' && get(activeTab) === 'files') activeTab.set('account');
}
