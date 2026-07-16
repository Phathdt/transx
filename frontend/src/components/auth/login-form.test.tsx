import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LoginForm } from '#/components/auth/login-form'

const mutate = vi.fn()
const navigate = vi.fn()

vi.mock('#/hooks/use-auth', () => ({
  useAuth: () => ({ login: { mutate, isPending: false } }),
}))

vi.mock('react-router', () => ({
  useNavigate: () => navigate,
}))

beforeEach(() => {
  mutate.mockReset()
  navigate.mockReset()
})

describe('LoginForm', () => {
  it('shows validation errors for empty submit', async () => {
    const user = userEvent.setup()
    render(<LoginForm />)
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Email is required')).toBeTruthy()
    expect(screen.getByText('Password is required')).toBeTruthy()
    expect(mutate).not.toHaveBeenCalled()
  })

  it('rejects an invalid email', async () => {
    const user = userEvent.setup()
    render(<LoginForm />)
    await user.type(screen.getByLabelText('Email'), 'not-an-email')
    await user.type(screen.getByLabelText('Password'), 'password123')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('Enter a valid email')).toBeTruthy()
    expect(mutate).not.toHaveBeenCalled()
  })

  it('submits valid credentials and surfaces an API error', async () => {
    mutate.mockImplementation((_values, opts) => {
      opts.onError({ message: 'Invalid credentials', status: 401 })
    })
    const user = userEvent.setup()
    render(<LoginForm />)
    await user.type(screen.getByLabelText('Email'), 'alice@transx.dev')
    await user.type(screen.getByLabelText('Password'), 'password123')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(mutate).toHaveBeenCalledTimes(1))
    expect(await screen.findByText('Invalid credentials')).toBeTruthy()
    expect(navigate).not.toHaveBeenCalled()
  })
})
