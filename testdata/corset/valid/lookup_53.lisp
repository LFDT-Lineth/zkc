;; As lookup_26, but without selectors.
(defcolumns (X :i17) (Y :i16))
(deflookup (test :unchecked) ((_ X)) ((_ Y)))
