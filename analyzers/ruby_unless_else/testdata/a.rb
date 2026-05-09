def f(x)
  unless x.nil?      # want "`unless` with an `else` branch"
    a
  else
    b
  end
end

def g(y)
  unless y.empty?    # OK: no else clause
    c
  end
end

def h(z)
  if z.nil?          # OK: regular if/else
    d
  else
    e
  end
end
