import { IconFontFill } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import { appLogo, appName, projectUrl } from '@/conf.json';
import {
  useFetchUserInfo,
  useListTenant,
} from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { TenantRole } from '@/pages/user-setting/constants';
import { Routes } from '@/routes';
import {
  LucideChevronDown,
  LucideCircleHelp,
  LucideLanguages,
} from 'lucide-react';
import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';
import { BellButton } from './bell-button';
import { DesktopNavbar, MobileNavbar } from './global-navbar';
import { MobileMenuFooter } from './mobile-menu-footer';
import ThemeButton from './theme-button';
import { useHeaderNavLayout } from './use-header-nav-layout';

import { supportedLanguages } from '@/locales/config';

export function Header({
  className,
  ...props
}: React.HTMLAttributes<HTMLElement>) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const changeLanguage = useChangeLanguage();

  const {
    data: { language = 'en', avatar, nickname },
  } = useFetchUserInfo();

  const { data: tenantData } = useListTenant();
  const hasNotification = useMemo(
    () => tenantData?.some((x) => x.role === TenantRole.Invite),
    [tenantData],
  );

  const currentLanguage = supportedLanguages.find((x) => x.code === language);

  const {
    headerRef,
    logoRef,
    expandedRightMeasureRef,
    navMeasureRef,
    isCompact,
  } = useHeaderNavLayout(`${hasNotification}-${language}`);

  const brand = (
    <Link
      to={Routes.Root}
      aria-current={pathname === Routes.Root ? 'page' : undefined}
      className="flex shrink-0 items-center gap-2.5 rounded-lg pe-1 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-accent-primary/15"
    >
      <img src={appLogo} alt="" className="size-8" />
      <span className="hidden whitespace-nowrap text-[15px] font-semibold tracking-tight text-text-primary sm:inline">
        {appName}
      </span>
    </Link>
  );

  const languageLabel = currentLanguage?.displayName;

  return (
    <>
      <header
        ref={headerRef}
        key="app-navbar"
        className={cn(
          'relative z-20 flex h-14 w-full min-w-0 items-center gap-3 border-b-0.5 border-border-button bg-bg-base/90 backdrop-blur-md sm:gap-6',
          className,
        )}
        {...props}
      >
        <div className="inline-flex shrink-0 items-center gap-1">
          {isCompact && (
            <MobileNavbar
              renderFooter={(close) => <MobileMenuFooter onClose={close} />}
            />
          )}
          <div ref={logoRef} className="inline-flex shrink-0 items-center">
            {brand}
          </div>
        </div>

        {!isCompact && (
          <div className="flex min-w-0 flex-1 overflow-hidden">
            <DesktopNavbar />
          </div>
        )}

        {isCompact && <div className="flex-1" aria-hidden />}

        <div
          className="flex shrink-0 items-center justify-end gap-0.5 text-text-secondary"
          data-testid="auth-status"
        >
          {!isCompact && (
            <Button
              asLink
              variant="ghost"
              size="icon"
              to={projectUrl}
              target="_blank"
              rel="noreferrer noopener"
              aria-label={t('header.github')}
              title={t('header.github')}
            >
              <IconFontFill name="GitHub" className="size-4" />
            </Button>
          )}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                className={cn('h-8 gap-1.5 px-2', isCompact && 'w-8 px-0')}
                aria-label={languageLabel}
              >
                <LucideLanguages className="size-4" />
                {!isCompact && (
                  <>
                    <span className="text-sm">{languageLabel}</span>
                    <LucideChevronDown className="size-3.5 opacity-60" />
                  </>
                )}
              </Button>
            </DropdownMenuTrigger>

            <DropdownMenuContent align="end">
              {supportedLanguages.map((x) => (
                <DropdownMenuItem
                  key={x.code}
                  onClick={() => changeLanguage(x.code)}
                  className={cn(
                    x.code === language && 'font-medium text-text-primary',
                  )}
                >
                  {x.displayName}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          {!isCompact && (
            <>
              <Button
                asLink
                variant="ghost"
                size="icon"
                to={`${projectUrl}#readme`}
                target="_blank"
                rel="noreferrer noopener"
                aria-label={t('header.help')}
                title={t('header.help')}
              >
                <LucideCircleHelp className="size-4" />
              </Button>

              {hasNotification && <BellButton className="!size-8" />}
            </>
          )}

          <ThemeButton className="!size-8" />

          <span
            className="mx-2 h-5 w-px bg-border-button max-sm:hidden"
            aria-hidden
          />

          <Link
            to={Routes.UserSetting}
            className="relative flex shrink-0 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-accent-primary/15"
            data-testid="settings-entrypoint"
            aria-label={t('header.setting')}
            title={nickname || t('header.setting')}
          >
            <RAGFlowAvatar
              name={nickname}
              avatar={avatar}
              isPerson
              className="size-8"
            />
          </Link>
        </div>
      </header>

      <div
        className="pointer-events-none invisible fixed -left-[9999px] top-0"
        aria-hidden
      >
        <div ref={navMeasureRef}>
          <DesktopNavbar />
        </div>
        <div
          ref={expandedRightMeasureRef}
          className="inline-flex shrink-0 items-center justify-end gap-0.5"
        >
          <span className="size-8" />
          <span className="inline-flex h-8 items-center gap-1.5 px-2 text-sm">
            <LucideLanguages className="size-4" />
            {languageLabel}
            <LucideChevronDown className="size-3.5" />
          </span>
          <span className="size-8" />
          {hasNotification && <span className="size-8" />}
          <span className="size-8" />
          <span className="mx-2 h-5 w-px" />
          <span className="size-8" />
        </div>
      </div>
    </>
  );
}
