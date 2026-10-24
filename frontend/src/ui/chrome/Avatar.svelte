<script lang="ts">
    import { avatarViewFor, type AvatarUser } from '../../modules/avatar';

    interface Props {
        user: AvatarUser | null;
        large?: boolean;
    }

    let { user, large = false }: Props = $props();

    const view = $derived(avatarViewFor(user));
    // Only a photo sets an inline style; initials take the neutral fallback from account.css.
    const style = $derived(
        view.photoUrl
            ? `background-image: url("${view.photoUrl}"); background-size: cover; background-position: center;`
            : '',
    );
</script>

<span class={`profile-avatar${large ? ' profile-avatar-lg' : ''}`} aria-hidden="true" {style}>{view.initials ?? ''}</span>
