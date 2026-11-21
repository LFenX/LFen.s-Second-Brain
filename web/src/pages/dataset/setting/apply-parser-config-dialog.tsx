import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import message from '@/components/ui/message';
import { useApplyDatasetParserConfig } from '@/hooks/use-apply-dataset-parser-config';
import { useTranslation } from 'react-i18next';

type ApplyParserConfigDialogProps = {
  datasetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ApplyParserConfigDialog({
  datasetId,
  open,
  onOpenChange,
}: ApplyParserConfigDialogProps) {
  const { t } = useTranslation();
  const { mutateAsync, isPending, error, reset } =
    useApplyDatasetParserConfig(datasetId);

  const handleCancel = () => {
    if (isPending) {
      return;
    }
    reset();
    onOpenChange(false);
  };

  const handleApply = async () => {
    if (isPending) {
      return;
    }
    try {
      await mutateAsync();
      message.success(t('knowledgeConfiguration.parserSettingsApplied'));
      onOpenChange(false);
    } catch {
      // Keep the dialog open so Yes can retry the operation.
    }
  };

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) handleCancel();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t('knowledgeConfiguration.applyParserSettingsTitle')}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t('knowledgeConfiguration.parserSettingsChangeWarning')}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <p role="alert" className="text-state-error text-sm">
            {t('knowledgeConfiguration.parserSettingsApplyFailed')}{' '}
            {error.message}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogAction
            className="bg-bg-input text-text-secondary"
            disabled={isPending}
            onClick={(event) => {
              event.preventDefault();
              void handleApply();
            }}
          >
            {t('common.yes')}
          </AlertDialogAction>
          <AlertDialogCancel
            className="bg-text-primary text-bg-base"
            disabled={isPending}
          >
            {t('common.no')}
          </AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
