import { render, screen } from '@testing-library/react';
import { FileUpload, useFileUpload } from '../file-upload';

jest.mock('lucide-react', () => ({}));

function FileNames() {
  const names = useFileUpload((state) =>
    [...state.files.keys()].map((file) => file.name).join(', '),
  );
  return <output aria-label="Selected files">{names}</output>;
}

it('keeps a stable upload store and updates its cached selection snapshot', () => {
  const first = new File(['first'], 'first.txt');
  const second = new File(['second'], 'second.txt');
  const { rerender } = render(
    <FileUpload value={[first]}>
      <FileNames />
    </FileUpload>,
  );
  expect(screen.getByLabelText('Selected files')).toHaveTextContent(
    'first.txt',
  );
  rerender(
    <FileUpload value={[second]}>
      <FileNames />
    </FileUpload>,
  );
  expect(screen.getByLabelText('Selected files')).toHaveTextContent(
    'second.txt',
  );
  expect(screen.getByLabelText('Selected files')).not.toHaveTextContent(
    'first.txt',
  );
});
