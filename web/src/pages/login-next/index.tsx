import { appLogo, appName } from '@/conf.json';
import { useAuth } from '@/hooks/auth-hooks';
import { useLogin, useRegister } from '@/hooks/use-login-request';
import { useSystemConfig } from '@/hooks/use-system-request';
import { rsaPsw } from '@/utils';
import { useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { BookOpenCheck, Share2, ShieldCheck } from 'lucide-react';

import { Button, ButtonLoading } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, UseFormReturn } from 'react-hook-form';
import { z } from 'zod';
import { NICKNAME_PATTERN } from '../user-setting/profile/constants';
import FlipCard3D, { FlipFaceContext } from './card';
import './index.less';

type LoginFormContentProps = {
  isLoginPage: boolean;
  title: string;
  form: UseFormReturn<any>;
  loading: boolean;
  onCheck: (params: any) => Promise<void>;
  changeTitle: () => void;
  registerEnabled: boolean;
  t: ReturnType<typeof useTranslation>['t'];
};

function LoginFormContent({
  isLoginPage,
  title,
  form,
  loading,
  onCheck,
  changeTitle,
  registerEnabled,
  t,
}: LoginFormContentProps) {
  const face = useContext(FlipFaceContext);
  const isActiveFace = isLoginPage ? face === 'front' : face === 'back';

  return (
    <div className="flex w-full flex-col items-center justify-center">
      <div className="w-full max-w-[420px] rounded-3xl border-0.5 border-border-button bg-bg-component px-8 pb-6 pt-8 shadow-soft">
        <div className="mb-7 space-y-1.5">
          <h2 className="text-[22px] font-semibold tracking-tight text-text-primary">
            {title === 'login' ? t('loginTitle') : t('signUpTitle')}
          </h2>
          <p className="text-sm text-text-secondary">
            {title === 'login'
              ? t('loginDescription')
              : t('registerDescription')}
          </p>
        </div>
        <Form {...form}>
          <form
            className="flex flex-col gap-5 text-text-primary"
            data-testid="auth-form"
            data-active={isActiveFace ? 'true' : undefined}
            onSubmit={form.handleSubmit(onCheck)}
          >
            <FormField
              control={form.control}
              name="email"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('emailLabel')}</FormLabel>
                  <FormControl>
                    <Input
                      data-testid="auth-email"
                      placeholder={t('emailPlaceholder')}
                      autoComplete="email"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {title === 'register' && (
              <FormField
                control={form.control}
                name="nickname"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('nicknameLabel')}</FormLabel>
                    <FormControl>
                      <Input
                        data-testid="auth-nickname"
                        placeholder={t('nicknamePlaceholder')}
                        autoComplete="username"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('passwordLabel')}</FormLabel>
                  <FormControl>
                    <div className="relative">
                      <Input
                        data-testid="auth-password"
                        type={'password'}
                        placeholder={t('passwordPlaceholder')}
                        autoComplete={
                          title === 'login'
                            ? 'current-password'
                            : 'new-password'
                        }
                        {...field}
                      />
                    </div>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {title === 'login' && (
              <FormField
                control={form.control}
                name="remember"
                render={({ field }) => (
                  <FormItem>
                    <div className="flex gap-2 group">
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(checked);
                          }}
                          className="group-hover:border-border-default group-hover:bg-border-button"
                        />
                      </FormControl>
                      <FormLabel
                        className={cn('cursor-pointer', {
                          'text-text-disabled': !field.value,
                          'text-text-primary': field.value,
                        })}
                      >
                        {t('rememberMe')}
                      </FormLabel>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <ButtonLoading
              data-testid="auth-submit"
              type="submit"
              loading={loading}
              className="my-4 w-full"
            >
              {title === 'login' ? t('login') : t('continue')}
            </ButtonLoading>
          </form>
        </Form>

        {title === 'login' && registerEnabled && (
          <div className="mt-10 text-right">
            <p className="text-text-disabled text-sm">
              {t('signInTip')}
              <Button
                data-testid="auth-toggle-register"
                variant={'transparent'}
                onClick={changeTitle}
                className="text-accent-primary/90 hover:text-accent-primary hover:bg-transparent font-medium border-none transition-colors duration-200"
              >
                {t('signUp')}
              </Button>
            </p>
          </div>
        )}
        {title === 'register' && (
          <div className="mt-10 text-right">
            <p className="text-text-disabled text-sm">
              {t('signUpTip')}
              <Button
                data-testid="auth-toggle-login"
                variant={'transparent'}
                onClick={changeTitle}
                className="text-accent-primary/90 hover:text-accent-primary hover:bg-transparent font-medium border-none transition-colors duration-200"
              >
                {t('login')}
              </Button>
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

const Login = () => {
  const [title, setTitle] = useState('login');
  const navigate = useNavigate();
  const { login, loading: signLoading } = useLogin();
  const { register, loading: registerLoading } = useRegister();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const { t: tSetting } = useTranslation('translation', {
    keyPrefix: 'setting',
  });
  const [isLoginPage, setIsLoginPage] = useState(true);

  const loading = signLoading || registerLoading;
  const { config } = useSystemConfig();
  const registerEnabled =
    config?.registerEnabled === 1 || config?.registerEnabled === true;

  const { isLogin } = useAuth();
  useEffect(() => {
    if (isLogin) {
      navigate('/');
    }
  }, [isLogin, navigate]);

  const changeTitle = () => {
    setIsLoginPage(title !== 'login');
    if (title === 'login' && !registerEnabled) {
      return;
    }

    setTimeout(() => {
      setTitle(title === 'login' ? 'register' : 'login');
    }, 200);
  };

  const FormSchema = z
    .object({
      nickname: z.string().optional(),
      email: z
        .string()
        .email()
        .min(1, { message: t('emailPlaceholder') }),
      password: z.string().min(1, { message: t('passwordPlaceholder') }),
      remember: z.boolean().optional(),
    })
    .superRefine((data, ctx) => {
      if (title !== 'register') return;
      if (!data.nickname) {
        ctx.addIssue({
          path: ['nickname'],
          message: 'nicknamePlaceholder',
          code: z.ZodIssueCode.custom,
        });
        return;
      }
      if (!NICKNAME_PATTERN.test(data.nickname)) {
        ctx.addIssue({
          path: ['nickname'],
          message: tSetting('usernameInvalidCharacters'),
          code: z.ZodIssueCode.custom,
        });
      }
    });
  type FormValues = z.infer<typeof FormSchema>;
  const form = useForm<FormValues>({
    defaultValues: {
      nickname: '',
      email: '',
      password: '',
      remember: false,
    },
    resolver: zodResolver(FormSchema),
  });

  const onCheck = async (params: FormValues) => {
    try {
      const rsaPassWord = rsaPsw(params.password) as string;

      if (title === 'login') {
        const code = await login({
          email: `${params.email}`.trim(),
          password: rsaPassWord,
        });
        if (code === 0) {
          navigate('/');
        }
      } else {
        const code = await register({
          nickname: params.nickname ?? '',
          email: params.email,
          password: rsaPassWord,
        });
        if (code === 0) {
          setTitle('login');
        }
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  const features = [
    { icon: BookOpenCheck, text: t('featureCite') },
    { icon: Share2, text: t('featureShare') },
    { icon: ShieldCheck, text: t('featureLocal') },
  ];

  const brand = (
    <div className="flex items-center gap-2.5">
      <img src={appLogo} alt="" className="size-9" />
      <span className="text-[17px] font-semibold tracking-tight">
        {appName}
      </span>
    </div>
  );

  return (
    <div className="flex min-h-full w-full overflow-auto bg-bg-base text-text-primary">
      <aside className="hidden w-[42%] max-w-[560px] shrink-0 flex-col justify-between border-e-0.5 border-border-button bg-bg-card px-12 py-10 lg:flex">
        {brand}
        <div className="space-y-6">
          <h1 className="text-[34px] font-semibold leading-tight tracking-tight">
            {t('title')}
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-text-secondary">
            {t('description')}
          </p>
          <ul className="space-y-3">
            {features.map(({ icon: Icon, text }) => (
              <li
                key={text}
                className="flex items-center gap-3 text-sm text-text-primary"
              >
                <span className="flex size-8 shrink-0 items-center justify-center rounded-xl bg-bg-component shadow-[0_0_0_0.5px_var(--border-button)]">
                  <Icon className="size-4 text-accent-primary" />
                </span>
                {text}
              </li>
            ))}
          </ul>
        </div>
        <p className="text-xs text-text-disabled">© {appName}</p>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col items-center justify-center gap-8 px-5 py-10">
        <div className="lg:hidden">{brand}</div>
        <div className="relative h-[620px] w-full max-w-[420px]">
          <FlipCard3D isLoginPage={isLoginPage}>
            <LoginFormContent
              isLoginPage={isLoginPage}
              title={title}
              form={form}
              loading={loading}
              onCheck={onCheck}
              changeTitle={changeTitle}
              registerEnabled={registerEnabled}
              t={t}
            />
          </FlipCard3D>
        </div>
      </main>
    </div>
  );
};

export default Login;
