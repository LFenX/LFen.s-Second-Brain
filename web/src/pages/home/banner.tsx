import { Card, CardContent } from '@/components/ui/card';
import { appName } from '@/conf.json';
import { ArrowRight, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

function BannerCard() {
  return (
    <Card className="w-auto border-none h-3/4">
      <CardContent className="p-4">
        <span className="inline-block bg-backgroundCoreWeak rounded-sm px-1 text-xs">
          System
        </span>
        <div className="flex mt-1 gap-4">
          <span className="text-lg truncate">Setting up your LLM</span>
          <ArrowRight />
        </div>
      </CardContent>
    </Card>
  );
}

export function Banner() {
  return (
    <section className="bg-[url('@/assets/banner.png')] bg-cover h-28 rounded-2xl  my-8 flex gap-8 justify-between">
      <div className="h-full text-3xl font-bold items-center inline-flex ml-6">
        Welcome to {appName}
      </div>
      <div className="flex justify-between items-center gap-4 mr-5">
        <BannerCard></BannerCard>
        <BannerCard></BannerCard>
        <BannerCard></BannerCard>
        <button
          type="button"
          className="relative p-1 hover:bg-white/10 rounded-full transition-colors"
        >
          <X className="w-6 h-6 text-white" />
        </button>
      </div>
    </section>
  );
}

export function NextBanner() {
  const { t, i18n } = useTranslation();
  return (
    <h1
      className="text-[40px] leading-tight tracking-tight text-left"
      dir={i18n.language?.startsWith('ar') ? 'rtl' : 'ltr'}
    >
      <span className="font-normal text-text-secondary">
        {t('header.welcome')}{' '}
      </span>
      <span className="font-semibold text-text-primary">{appName}</span>
    </h1>
  );
}
