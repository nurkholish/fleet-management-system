import React from 'react'

export default class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, info) {
    console.error('[ErrorBoundary]', error, info)
  }

  handleReset = () => this.setState({ hasError: false, error: null })
  handleReload = () => window.location.reload()

  render() {
    if (this.state.hasError) {
      return (
        <div className="error-boundary">
          <div className="error-boundary__card">
            <div className="error-boundary__icon" aria-hidden="true">
              ⚠️
            </div>
            <h2 className="error-boundary__title">Something went wrong</h2>
            <p className="error-boundary__message">
              {this.state.error?.message ?? 'Unknown error'}
            </p>
            <details className="error-boundary__details">
              <summary>Technical details</summary>
              <pre>{String(this.state.error?.stack ?? this.state.error)}</pre>
            </details>
            <div className="error-boundary__actions">
              <button onClick={this.handleReset}>Try Again</button>
              <button className="btn-secondary" onClick={this.handleReload}>
                Reload
              </button>
            </div>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}