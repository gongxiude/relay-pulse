import { useTranslation } from 'react-i18next';

export type ChannelType = 'official' | 'reverse' | 'mixed' | 'unknown';

/** Parse O-/R-/M- prefix from channel identifier. Returns 'unknown' for unrecognized formats. */
export function parseChannelType(channel?: string | null): ChannelType | null {
  if (!channel) return null;
  const prefix = channel.charAt(0).toUpperCase();
  if (channel.charAt(1) === '-') {
    if (prefix === 'O') return 'official';
    if (prefix === 'R') return 'reverse';
    if (prefix === 'M') return 'mixed';
  }
  return 'unknown';
}

export function normalizeChannelType(channelType?: string | null): ChannelType | null {
  if (!channelType) return null;
  if (channelType === 'official') return 'official';
  if (channelType === 'reverse') return 'reverse';
  if (channelType === 'mixed') return 'mixed';
  if (channelType === 'unknown') return 'unknown';
  return null;
}

// Solid five-pointed star (official/certified)
function OfficialIcon() {
  return (
    <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.27 5.82 21 7 14.14l-5-4.87 6.91-1.01L12 2z"
        className="fill-accent"
        opacity="0.55"
      />
    </svg>
  );
}

// Lightning bolt (reverse/unofficial)
function ReverseIcon() {
  return (
    <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M13 2L4.5 13H12l-1 9L19.5 11H12l1-9z"
        className="fill-warning"
        opacity="0.55"
      />
    </svg>
  );
}

// Two overlapping cards (mixed sources)
function MixedIcon() {
  return (
    <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="3" y="6" width="12" height="14" rx="2" className="fill-warning" opacity="0.25" />
      <rect x="9" y="4" width="12" height="14" rx="2" className="fill-warning" opacity="0.55" />
    </svg>
  );
}

// Question mark circle (unknown/unclassified)
function UnknownIcon() {
  return (
    <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="10" className="fill-muted" opacity="0.3" />
      <text x="12" y="17" textAnchor="middle" className="fill-muted" fontSize="14" fontWeight="bold">?</text>
    </svg>
  );
}

interface ChannelTypeIconProps {
  channel?: string | null;
  channelType?: string | null;
}

/** Renders a channel type icon based on O-/R-/M- prefix. Unrecognized formats show unknown icon. */
export function ChannelTypeIcon({ channel, channelType }: ChannelTypeIconProps) {
  const { t } = useTranslation();
  const type = normalizeChannelType(channelType) || parseChannelType(channel);
  if (!type) return null;

  const label = t(`table.channelType.${type}`);
  const desc = t(`table.channelType.${type}Desc`);
  const tip = desc !== label ? `${label} — ${desc}` : label;

  const icons: Record<ChannelType, React.JSX.Element> = {
    official: <OfficialIcon />,
    reverse: <ReverseIcon />,
    mixed: <MixedIcon />,
    unknown: <UnknownIcon />,
  };

  return (
    <span className="inline-flex flex-shrink-0" title={tip} role="img" aria-label={label}>
      {icons[type]}
    </span>
  );
}
