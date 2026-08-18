(defconst CHAIN 1)

(defconst
  LIMIT_0 1000
  LIMIT_1 1100)

(defcolumns (ST :i4) (X :i16))

(defconstraint c1 (:guard ST) (== X (+
           ;; CHAIN=0
           (* (- 1 CHAIN) LIMIT_0)
           ;; CHAIN=1
           (* CHAIN LIMIT_1))))
