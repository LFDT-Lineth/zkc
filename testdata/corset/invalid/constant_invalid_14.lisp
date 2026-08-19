;;error:5:41-42:not permitted in pure context
;;error:5:67-68:not permitted in pure context
;;error:5:22-71:expected bool, found i20
(defcolumns (CT :i4) (X :i16))
(defconstraint c1 () (* (- CT (shift CT X)) (- (+ CT X) (shift CT X))))
