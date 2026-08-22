import Image from 'next/image';

export function PokerLogo({className = '', size = 38, priority = false}: {
  className?: string;
  size?: number;
  priority?: boolean;
}) {
  // Requested at 3x the display size (every call site sizes the box itself via
  // CSS): Safari rasterizes an <img>-embedded SVG once at these attributes,
  // ignoring devicePixelRatio, so a 1x-sized source reads blurry on Retina iPhones.
  return <Image
    src="/svgs/logo.svg"
    alt=""
    aria-hidden="true"
    width={size * 3}
    height={size * 3}
    className={className}
    priority={priority}
  />;
}
