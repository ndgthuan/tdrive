<script lang="ts">
    import ArrowRightIcon from 'phosphor-svelte/lib/ArrowRightIcon';
    import ChevronLeftIcon from 'phosphor-svelte/lib/CaretLeftIcon';
    import ChevronRightIcon from 'phosphor-svelte/lib/CaretRightIcon';
    import DownloadIcon from 'phosphor-svelte/lib/DownloadSimpleIcon';
    import EyeIcon from 'phosphor-svelte/lib/EyeIcon';
    import EyeOffIcon from 'phosphor-svelte/lib/EyeSlashIcon';
    import FingerprintIcon from 'phosphor-svelte/lib/FingerprintIcon';
    import InfoIcon from 'phosphor-svelte/lib/InfoIcon';
    import LockIcon from 'phosphor-svelte/lib/LockIcon';
    import XIcon from 'phosphor-svelte/lib/XIcon';
    import { t_store } from '../../i18n/i18n';
</script>

<div id="preview-shell" class="preview-shell" role="dialog" aria-modal="true" aria-labelledby="preview-filename">
    <div class="preview-chrome">
        <div id="preview-filename" class="preview-filename" aria-live="polite"></div>
        <div class="preview-chrome-actions">
            <button id="preview-info-btn" class="preview-icon-btn" type="button" aria-label={$t_store('viewer.photo_info_label')} aria-pressed="false" title={$t_store('viewer.info_tooltip')}>
                <InfoIcon size={24} aria-hidden="true" />
            </button>
            <button id="preview-download" class="preview-icon-btn" type="button" aria-label={$t_store('viewer.download_original')} title={$t_store('viewer.download_original')}>
                <DownloadIcon size={24} aria-hidden="true" />
            </button>
            <button id="preview-close" class="preview-close-btn" type="button" aria-label={$t_store('viewer.close_preview')} title={$t_store('viewer.close_preview')}>
                <XIcon size={24} aria-hidden="true" />
            </button>
        </div>
    </div>

    <button id="preview-prev" class="preview-nav preview-nav-prev" type="button" aria-label={$t_store('viewer.previous_image')} hidden>
        <ChevronLeftIcon size={24} aria-hidden="true" />
    </button>
    <button id="preview-next" class="preview-nav preview-nav-next" type="button" aria-label={$t_store('viewer.next_image')} hidden>
        <ChevronRightIcon size={24} aria-hidden="true" />
    </button>

    <div id="preview-counter" class="preview-counter" hidden></div>

    <div id="preview-stage" class="preview-content">
        <img id="preview-thumbnail" class="preview-thumbnail" src="" alt="" aria-hidden="true" hidden>
        <img id="preview-image" class="preview-original" src="" alt="" hidden>
        <div id="preview-loading" class="preview-loading" role="status" aria-label={$t_store('viewer.loading_original')} aria-hidden="true" style="display: none;">
            <div class="preview-loading-track" aria-hidden="true">
                <div id="preview-loading-fill" class="preview-loading-fill"></div>
            </div>
        </div>
        <div id="preview-error" class="preview-error" style="display: none;" role="alert"></div>
    </div>

    <aside id="preview-info" class="preview-info" aria-label={$t_store('viewer.photo_info_label')} aria-hidden="true" inert>
        <button id="preview-info-close" class="preview-info-close" type="button" aria-label={$t_store('viewer.close_info')} title={$t_store('viewer.close_info')}>
            <XIcon size={14} aria-hidden="true" weight="bold" />
        </button>
        <div id="preview-info-body"></div>
    </aside>

    <div id="preview-locked" class="preview-locked" style="display: none;" role="group" aria-label={$t_store('viewer.locked_photo_label')}>
        <div class="preview-unlock">
            <div class="preview-unlock-bar">
                <LockIcon size={24} class="preview-unlock-lock" aria-hidden="true" />
                <input id="preview-locked-input" type="password" placeholder={$t_store('viewer.password')} autocomplete="current-password" spellcheck="false" aria-label={$t_store('viewer.encryption_password')}>
                <button id="preview-locked-eye" class="preview-unlock-eye" type="button" aria-label={$t_store('viewer.show_password')} aria-pressed="false">
                    <EyeIcon size={24} class="preview-eye-show" aria-hidden="true" />
                    <EyeOffIcon size={24} class="preview-eye-hide" aria-hidden="true" />
                </button>
                <button id="preview-locked-unlock" class="preview-unlock-go" type="button" aria-label={$t_store('viewer.unlock')}>
                    <ArrowRightIcon size={24} aria-hidden="true" />
                </button>
            </div>
            <div id="preview-locked-error" class="preview-unlock-error" style="display: none;" role="alert"></div>
            <div id="preview-locked-hint" class="preview-unlock-hint" style="display: none;">{$t_store('viewer.hint_prefix')} <span id="preview-locked-hint-text"></span></div>
            <button id="preview-locked-biometric" class="preview-unlock-biometric" type="button" hidden>
                <FingerprintIcon size={20} aria-hidden="true" />
                <span id="preview-locked-biometric-label">{$t_store('viewer.use_biometrics')}</span>
            </button>
            <p class="preview-unlock-biometric-help">{$t_store('viewer.unlock_help')}</p>
        </div>
    </div>
</div>

<style>
    :global(#preview-locked-eye .preview-eye-hide) {
        display: none;
    }

    :global(#preview-locked-eye[aria-pressed="true"] .preview-eye-show) {
        display: none;
    }

    :global(#preview-locked-eye[aria-pressed="true"] .preview-eye-hide) {
        display: block;
    }
</style>
