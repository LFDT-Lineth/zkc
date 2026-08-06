(defcolumns (ST :i4) (X :i16) (Y :i20) (STX :i20))
;; STX holds the value of the expression (* ST (+ 1 X))
(defconstraint stx_def () (== STX (* ST (+ 1 X))))
(deflookup test (Y) (STX))
