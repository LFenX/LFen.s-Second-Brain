import { fireEvent, render, screen } from '@testing-library/react';
import { BoolSegmented } from '../bool-segmented';

describe('BoolSegmented', () => {
  it('renders a false value and emits booleans when either option is selected', () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <BoolSegmented value={false} onChange={onChange} />,
    );
    expect(screen.getByRole('button', { name: 'False' }).className).toContain(
      'text-text-primary',
    );
    fireEvent.click(screen.getByRole('button', { name: 'True' }));
    expect(onChange).toHaveBeenLastCalledWith(true);
    rerender(<BoolSegmented value={true} onChange={onChange} />);
    expect(screen.getByRole('button', { name: 'True' }).className).toContain(
      'text-text-primary',
    );
    fireEvent.click(screen.getByRole('button', { name: 'False' }));
    expect(onChange).toHaveBeenLastCalledWith(false);
  });

  it('uses a false default without converting it into an empty selection', () => {
    render(<BoolSegmented defaultValue={false} />);
    expect(screen.getByRole('button', { name: 'False' }).className).toContain(
      'text-text-primary',
    );
  });
});
