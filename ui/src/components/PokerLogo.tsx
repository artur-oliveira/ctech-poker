import Image from 'next/image';

export function PokerLogo({className = '', size = 38, priority = false}: {
  className?: string;
  size?: number;
  priority?: boolean;
}) {
  return <Image
    src="/svgs/logo.svg"
    alt=""
    aria-hidden="true"
    width={size}
    height={size}
    className={className}
    priority={priority}
  />;
}
